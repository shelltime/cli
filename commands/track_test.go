package commands

// Basic imports
import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

type trackTestSuite struct {
	suite.Suite
	baseTimeFolder        string
	originalStorageFolder string
}

// before each test
func (s *trackTestSuite) SetupSuite() {
	s.originalStorageFolder = model.COMMAND_BASE_STORAGE_FOLDER
	s.baseTimeFolder = strconv.FormatInt(time.Now().UnixNano(), 10)
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
}

func (s *trackTestSuite) TestMultipTrackWithPre() {
	cs := model.NewMockConfigService(s.T())
	mockedConfig := model.ShellTimeConfig{}
	cs.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)
	model.UserShellTimeConfig = mockedConfig
	configService = cs

	baseFolder := s.baseTimeFolder + "-withPre"
	model.InitFolder(baseFolder)

	p := os.ExpandEnv("$HOME/" + model.COMMAND_STORAGE_FOLDER)
	err := os.MkdirAll(p, os.ModePerm)
	assert.Nil(s.T(), err)

	times := 10

	var wg sync.WaitGroup
	wg.Add(times)

	sessionID := time.Now().Unix()

	for i := 0; i < times; i++ {
		command := []string{
			"mtt",
			"track",
			"-s=fish",
			fmt.Sprintf("-id=%d", sessionID),
			fmt.Sprintf("-cmd=cmd1 %d", i),
			"-p=pre",
		}
		go func(cmd []string) {
			err := commandTrack(trackActionContext(cmd))
			assert.Nil(s.T(), err)
			wg.Done()
		}(command)
	}

	wg.Wait()

	preFile := os.ExpandEnv("$HOME/" + model.COMMAND_PRE_STORAGE_FILE)
	content, err := os.ReadFile(preFile)
	assert.Nil(s.T(), err)

	lines := 0
	for _, byte := range content {
		if byte == '\n' {
			lines++
		}
	}
	assert.Equal(s.T(), times, lines)
}

func (s *trackTestSuite) TestTrackWithSendData() {
	reqCursor := make([]int64, 0)
	var cursorMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationHeader := r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		assert.Nil(s.T(), err)
		defer r.Body.Close()

		var payload model.PostTrackArgs

		err = json.Unmarshal(body, &payload)
		assert.Nil(s.T(), err)

		// allow first sync so no need to check the minimum length
		// assert.GreaterOrEqual(s.T(), len(payload.Data), 7)

		assert.Contains(s.T(), string(body), "fish")
		assert.EqualValues(s.T(), "CLI TOKEN001", authorizationHeader)
		w.WriteHeader(http.StatusNoContent)
		cursorMu.Lock()
		reqCursor = append(reqCursor, payload.CursorID)
		cursorMu.Unlock()
	}))
	defer server.Close()
	cs := model.NewMockConfigService(s.T())
	truthy := true
	mockedConfig := model.ShellTimeConfig{
		Token:       "TOKEN001",
		APIEndpoint: server.URL,
		FlushCount:  7,
		GCTime:      8,
		LogCleanup: &model.LogCleanup{
			Enabled:     &truthy,
			ThresholdMB: 100,
		},
	}
	cs.On("ReadConfigFile", mock.Anything).Return(mockedConfig, nil)
	model.UserShellTimeConfig = mockedConfig
	configService = cs

	baseFolder := s.baseTimeFolder + "-sendData"
	model.InitFolder(baseFolder)

	err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), model.COMMAND_STORAGE_FOLDER), os.ModePerm)
	assert.Nil(s.T(), err)

	app := &cli.App{
		// mtt for malamtime-testing
		Name:  "mtt2",
		Usage: "just for testing",
		Commands: []*cli.Command{
			TrackCommand,
			GCCommand,
		},
	}

	times := 16

	var wg sync.WaitGroup
	wg.Add(times)

	sessionID := time.Now().Unix()

	unfinishedCommand := []string{
		"mtt2",
		"track",
		"-s=fish",
		fmt.Sprintf("-id=%d", sessionID),
		"-cmd=unfinished_cmd kangjinloong",
		"-p=pre",
	}
	err = app.Run(unfinishedCommand)
	assert.Nil(s.T(), err)
	time.Sleep(time.Millisecond * 300)

	for i := 0; i < times; i++ {
		command := []string{
			"mtt2",
			"track",
			"-s=fish",
			fmt.Sprintf("-id=%d", sessionID),
			fmt.Sprintf("-cmd=cmd1 %d", i),
			"-p=pre",
		}
		postCommand := []string{
			"mtt2",
			"track",
			"-s=fish",
			fmt.Sprintf("-id=%d", sessionID),
			fmt.Sprintf("-cmd=cmd1 %d", i),
			"-p=post",
		}
		go func(cmd []string, pc []string) {
			err := commandTrack(trackActionContext(cmd))
			assert.Nil(s.T(), err)
			time.Sleep(time.Millisecond * 100)
			err = commandTrack(trackActionContext(pc))
			assert.Nil(s.T(), err)
			wg.Done()
		}(command, postCommand)
	}

	wg.Wait()

	assert.GreaterOrEqual(s.T(), len(reqCursor), 2)

	// Check the number of lines in the COMMAND_PRE_STORAGE_FILE
	preFile := os.ExpandEnv("$HOME/" + model.COMMAND_PRE_STORAGE_FILE)
	content, err := os.ReadFile(preFile)
	assert.Nil(s.T(), err)

	lines := 0
	for _, byte := range content {
		if byte == '\n' {
			lines++
		}
	}
	assert.Equal(s.T(), times+1, lines)

	// Check the number of lines in the COMMAND_POST_STORAGE_FILE
	postFile := os.ExpandEnv("$HOME/" + model.COMMAND_POST_STORAGE_FILE)
	postContent, err := os.ReadFile(postFile)
	assert.Nil(s.T(), err)

	postLines := 0
	for _, byte := range postContent {
		if byte == '\n' {
			postLines++
		}
	}
	assert.Equal(s.T(), times, postLines)

	// Check the CURSOR_FILE
	cursorFile := os.ExpandEnv("$HOME/" + model.COMMAND_CURSOR_STORAGE_FILE)
	cursorContent, err := os.ReadFile(cursorFile)
	assert.Nil(s.T(), err)

	var cursorValues []time.Time
	for _, line := range strings.Split(string(cursorContent), "\n") {
		if line != "" {
			nanoTime, err := strconv.ParseInt(line, 10, 64)
			assert.Nil(s.T(), err)
			cursorValues = append(cursorValues, time.Unix(0, nanoTime))
		}
	}
	assert.GreaterOrEqual(s.T(), len(cursorValues), 2)

	// Cursor values are written by concurrent goroutines, so their order in the
	// file reflects write order, not timestamp order. Assert the cursor advanced
	// over time by comparing the min and max values (order-independent) instead
	// of relying on the last line being the latest timestamp.
	minCursor, maxCursor := cursorValues[0], cursorValues[0]
	for _, value := range cursorValues {
		if value.Before(minCursor) {
			minCursor = value
		}
		if value.After(maxCursor) {
			maxCursor = value
		}
	}
	assert.True(s.T(), maxCursor.After(minCursor))

	reqCursorStr := strings.Join(strings.Fields(fmt.Sprint(reqCursor)), "\t")

	for _, value := range cursorValues {
		cursorInStr := strconv.FormatInt(value.UnixNano(), 10)
		assert.Contains(s.T(), string(postContent), cursorInStr)
		assert.Contains(s.T(), reqCursorStr, cursorInStr)
	}

	gcCmd := []string{
		"mtt2",
		"gc",
		"--slc",
	}

	gcErr := app.Run(gcCmd)
	assert.Nil(s.T(), gcErr)

	// Check the cursor file should be only one line
	cursorContent, err = os.ReadFile(cursorFile)
	assert.Nil(s.T(), err)
	s.T().Log("cursor content:", string(cursorContent))
	cursorLines := bytes.Split(cursorContent, []byte("\n"))
	assert.Len(s.T(), cursorLines, 1)

	// Check the pre file should be less than `times` of lines
	preContent, err := os.ReadFile(preFile)
	assert.Nil(s.T(), err)
	preContent = bytes.TrimSpace(preContent)
	s.T().Log("pre content:", string(preContent))
	preLines := bytes.Split(preContent, []byte("\n"))
	assert.LessOrEqual(s.T(), len(preLines)-1, times)
	assert.Contains(s.T(), string(preContent), "unfinished_cmd")

	// Check the post file should be less than `times` of lines
	postContent, err = os.ReadFile(postFile)
	assert.Nil(s.T(), err)
	postContent = bytes.TrimSpace(postContent)
	s.T().Log("post content:", string(postContent))
	postBytesLines := bytes.Split(postContent, []byte("\n"))
	assert.Less(s.T(), len(postBytesLines), times)
	assert.NotContains(s.T(), string(postContent), "unfinished_cmd")
}

func (s *trackTestSuite) TearDownSuite() {
	for _, suffix := range []string{"withPre", "sendData"} {
		folder := fmt.Sprintf(".shelltime-%s-%s", s.baseTimeFolder, suffix)
		assert.NoError(s.T(), os.RemoveAll(filepath.Join(os.Getenv("HOME"), folder)))
	}
	model.COMMAND_BASE_STORAGE_FOLDER = s.originalStorageFolder
	model.InitFolder("")
}

// Each hook process has independent parsing state. Exercise the same concurrent
// action calls here without sharing urfave/cli's mutable App or flag objects.
func trackActionContext(args []string) *cli.Context {
	flags := flag.NewFlagSet("track", flag.ContinueOnError)
	flags.String("shell", "", "")
	flags.Int64("sessionId", 0, "")
	flags.String("command", "", "")
	flags.String("phase", "", "")
	flags.Int("result", 0, "")
	flags.Int("ppid", 0, "")
	aliases := map[string]string{"s": "shell", "id": "sessionId", "cmd": "command", "p": "phase"}
	for _, arg := range args[2:] {
		name, value, ok := strings.Cut(strings.TrimPrefix(arg, "-"), "=")
		if !ok {
			panic("tracking argument is missing its value")
		}
		if canonical, ok := aliases[name]; ok {
			name = canonical
		}
		if err := flags.Set(name, value); err != nil {
			panic(err)
		}
	}
	ctx := cli.NewContext(nil, flags, nil)
	ctx.Context = context.Background()
	return ctx
}

func TestTrackTestSuite(t *testing.T) {
	suite.Run(t, new(trackTestSuite))
}
