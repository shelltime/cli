package commands

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

type pollStep struct {
	result model.AppApprovalPollResult
	err    error
}

// fakeAppApproval answers polls from a script, repeating the last step.
type fakeAppApproval struct {
	ticket model.AppApprovalTicket
	err    error
	steps  []pollStep
	polls  int
}

func (f *fakeAppApproval) Request(context.Context, string) (model.AppApprovalTicket, error) {
	return f.ticket, f.err
}

func (f *fakeAppApproval) Poll(_ context.Context, requestID, pollToken string) (model.AppApprovalPollResult, error) {
	if requestID != f.ticket.RequestID || pollToken != f.ticket.PollToken {
		return model.AppApprovalPollResult{}, errors.New("unexpected request")
	}
	step := f.steps[min(f.polls, len(f.steps)-1)]
	f.polls++
	return step.result, step.err
}

func approvedWith(token string) pollStep {
	result := model.AppApprovalPollResult{Status: model.AppApprovalApproved}
	result.OpenToken = &struct {
		ID    int    `json:"id"`
		Token string `json:"token"`
	}{ID: 1, Token: token}
	return pollStep{result: result}
}

func statusStep(status model.AppApprovalStatus) pollStep {
	return pollStep{result: model.AppApprovalPollResult{Status: status}}
}

var testTicket = model.AppApprovalTicket{RequestID: "rid", PollToken: "ptok", MatchCode: "42", ExpiresIn: 300, Interval: 3}

// noSleep records the waits instead of sleeping.
func noSleep(waits *[]time.Duration) func(context.Context, time.Duration) error {
	return func(ctx context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return ctx.Err()
	}
}

func TestWaitForAppApprovalApproved(t *testing.T) {
	svc := &fakeAppApproval{ticket: testTicket, steps: []pollStep{
		statusStep(model.AppApprovalPending),
		statusStep(model.AppApprovalPending),
		approvedWith("st_v1_token"),
	}}
	var waits []time.Duration

	token, err := waitForAppApproval(context.Background(), svc, testTicket, noSleep(&waits))
	require.NoError(t, err)
	assert.Equal(t, "st_v1_token", token)
	assert.Equal(t, 3, svc.polls)
	assert.Equal(t, []time.Duration{3 * time.Second, 3 * time.Second, 3 * time.Second}, waits, "uses the server's interval")
}

func TestWaitForAppApprovalDenied(t *testing.T) {
	svc := &fakeAppApproval{ticket: testTicket, steps: []pollStep{statusStep(model.AppApprovalDenied)}}
	var waits []time.Duration

	_, err := waitForAppApproval(context.Background(), svc, testTicket, noSleep(&waits))
	assert.ErrorIs(t, err, ErrAuthNotApproved)
	assert.Contains(t, err.Error(), "denied")
}

func TestWaitForAppApprovalExpired(t *testing.T) {
	svc := &fakeAppApproval{ticket: testTicket, steps: []pollStep{statusStep(model.AppApprovalExpired)}}
	var waits []time.Duration

	_, err := waitForAppApproval(context.Background(), svc, testTicket, noSleep(&waits))
	assert.ErrorIs(t, err, ErrAuthNotApproved)
	assert.Contains(t, err.Error(), "expired")
}

func TestWaitForAppApprovalRetriesServerErrors(t *testing.T) {
	svc := &fakeAppApproval{ticket: testTicket, steps: []pollStep{
		{err: errors.New("connection reset")},
		{err: &model.AppApprovalError{StatusCode: http.StatusBadGateway, Message: "Bad Gateway"}},
		approvedWith("st_v1_token"),
	}}
	var waits []time.Duration

	token, err := waitForAppApproval(context.Background(), svc, testTicket, noSleep(&waits))
	require.NoError(t, err)
	assert.Equal(t, "st_v1_token", token)
}

func TestWaitForAppApprovalStopsOnRefusal(t *testing.T) {
	svc := &fakeAppApproval{ticket: testTicket, steps: []pollStep{
		{err: &model.AppApprovalError{StatusCode: http.StatusBadRequest, Message: "Invalid sign-in request"}},
	}}
	var waits []time.Duration

	_, err := waitForAppApproval(context.Background(), svc, testTicket, noSleep(&waits))
	assert.ErrorIs(t, err, ErrAuthNotApproved)
	assert.Contains(t, err.Error(), "Invalid sign-in request")
	assert.Equal(t, 1, svc.polls)
}

func TestWaitForAppApprovalCancelled(t *testing.T) {
	svc := &fakeAppApproval{ticket: testTicket, steps: []pollStep{statusStep(model.AppApprovalPending)}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var waits []time.Duration

	_, err := waitForAppApproval(ctx, svc, testTicket, noSleep(&waits))
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, svc.polls)
}

func TestWaitForAppApprovalGivesUpAfterExpiry(t *testing.T) {
	ticket := testTicket
	ticket.ExpiresIn = -int(appApprovalPollGrace/time.Second) - 1
	svc := &fakeAppApproval{ticket: ticket, steps: []pollStep{statusStep(model.AppApprovalPending)}}
	var waits []time.Duration

	_, err := waitForAppApproval(context.Background(), svc, ticket, noSleep(&waits))
	assert.ErrorIs(t, err, ErrAuthNotApproved)
	assert.Equal(t, 1, svc.polls)
}

func TestPrintAppApprovalIntro(t *testing.T) {
	var out bytes.Buffer
	printAppApprovalIntro(&out, testTicket)

	assert.Contains(t, out.String(), "ShellTime app on your iPhone")
	assert.Contains(t, out.String(), "Code: 42")
	assert.Contains(t, out.String(), "expires in 5m0s")
}

func TestApplyTokenByAppApprovalRefused(t *testing.T) {
	original := newAppApprovalService
	t.Cleanup(func() { newAppApprovalService = original })
	newAppApprovalService = func(model.ShellTimeConfig) model.AppApprovalService {
		return &fakeAppApproval{err: &model.AppApprovalError{StatusCode: http.StatusNotFound, Message: "no iOS app"}}
	}

	_, err := ApplyTokenByAppApproval(context.Background(), model.ShellTimeConfig{}, "me@example.com")
	assert.ErrorIs(t, err, ErrAuthNotApproved)
}

func TestAuthRejectsTokenWithEmail(t *testing.T) {
	app := &cli.App{Commands: []*cli.Command{AuthCommand}}
	err := app.Run([]string{"shelltime", "auth", "--token", "abc", "--email", "me@example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "either --token or --email")
}

// useFakeAppApproval swaps in svc and a sleep that returns at once.
func useFakeAppApproval(t *testing.T, svc *fakeAppApproval) {
	originalService, originalSleep := newAppApprovalService, appApprovalSleep
	t.Cleanup(func() { newAppApprovalService, appApprovalSleep = originalService, originalSleep })
	newAppApprovalService = func(model.ShellTimeConfig) model.AppApprovalService { return svc }
	appApprovalSleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
}

func TestApplyTokenByAppApprovalApproved(t *testing.T) {
	svc := &fakeAppApproval{ticket: testTicket, steps: []pollStep{
		statusStep(model.AppApprovalPending),
		approvedWith("st_v1_token"),
	}}
	useFakeAppApproval(t, svc)

	token, err := ApplyTokenByAppApproval(context.Background(), model.ShellTimeConfig{}, "me@example.com")
	require.NoError(t, err)
	assert.Equal(t, "st_v1_token", token)
	assert.Equal(t, 2, svc.polls)
}

func TestApplyTokenByAppApprovalDenied(t *testing.T) {
	useFakeAppApproval(t, &fakeAppApproval{ticket: testTicket, steps: []pollStep{statusStep(model.AppApprovalDenied)}})

	_, err := ApplyTokenByAppApproval(context.Background(), model.ShellTimeConfig{}, "me@example.com")
	assert.ErrorIs(t, err, ErrAuthNotApproved)
}

func TestApplyTokenByAppApprovalUnreachable(t *testing.T) {
	useFakeAppApproval(t, &fakeAppApproval{err: errors.New("connection refused")})

	_, err := ApplyTokenByAppApproval(context.Background(), model.ShellTimeConfig{}, "me@example.com")
	assert.ErrorIs(t, err, ErrAuthNotApproved)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestAuthWithEmailSavesTheToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	useFakeAppApproval(t, &fakeAppApproval{ticket: testTicket, steps: []pollStep{approvedWith("st_v1_saved")}})

	app := &cli.App{Commands: []*cli.Command{AuthCommand}}
	require.NoError(t, app.Run([]string{"shelltime", "auth", "--email", "me@example.com"}))

	content, err := os.ReadFile(filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER, "config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "st_v1_saved")
}

func TestSleepContext(t *testing.T) {
	assert.NoError(t, sleepContext(context.Background(), time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, sleepContext(ctx, time.Hour), context.Canceled)
}

func TestWaitingIndicatorIsQuietWhenPiped(t *testing.T) {
	// go test's stdout is not a terminal, so this must be a no-op
	stop := startWaitingIndicator()
	stop()
}
