package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/malamtime/cli/model"
)

var (
	loadCursorAuthFunc   = model.LoadCursorAuth
	fetchCursorQuotaFunc = fetchCursorQuota
)

// CursorRateLimitData holds the parsed billing-cycle quota data from Cursor's DashboardService.
type CursorRateLimitData struct {
	Plan    string
	Windows []CursorRateLimitWindow
}

// CursorRateLimitWindow holds a single Cursor monthly quota window.
type CursorRateLimitWindow struct {
	LimitID               string
	LimitName             string
	UsagePercentage       float64
	ResetAt               int64 // Unix seconds
	WindowDurationMinutes int
}

type cursorCurrentPeriodUsageResponse struct {
	BillingCycleStart any                  `json:"billingCycleStart"`
	BillingCycleEnd   any                  `json:"billingCycleEnd"`
	PlanUsage         *cursorPlanUsageBody `json:"planUsage"`
}

type cursorPlanUsageBody struct {
	TotalPercentUsed *float64 `json:"totalPercentUsed"`
	AutoPercentUsed  *float64 `json:"autoPercentUsed"`
	APIPercentUsed   *float64 `json:"apiPercentUsed"`
}

// fetchCursorQuota calls Cursor's GetCurrentPeriodUsage RPC and maps it into quota windows.
func fetchCursorQuota(ctx context.Context, auth *model.CursorAuthData) (*CursorRateLimitData, error) {
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: model.HTTPTransport(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return fetchCursorQuotaFromEndpoint(ctx, auth, client)
}

func fetchCursorQuotaFromEndpoint(ctx context.Context, auth *model.CursorAuthData, client *http.Client) (*CursorRateLimitData, error) {
	if auth == nil || strings.TrimSpace(auth.AccessToken) == "" {
		return nil, model.ErrCursorAuthMissing
	}
	endpoint := strings.TrimRight(strings.TrimSpace(auth.Endpoint), "/")
	if endpoint == "" {
		endpoint = model.DefaultCursorAPIEndpoint
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint+"/aiserver.v1.DashboardService/GetCurrentPeriodUsage",
		bytes.NewReader([]byte("{}")),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("connect-protocol-version", "1")
	req.Header.Set("x-cursor-client-type", "cli")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "shelltime-daemon")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, model.ErrCursorTokenInvalid
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cursor quota API returned status %d", resp.StatusCode)
	}

	var body cursorCurrentPeriodUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode cursor quota response: %w", err)
	}

	return mapCursorCurrentPeriodUsage(&body)
}

func mapCursorCurrentPeriodUsage(body *cursorCurrentPeriodUsageResponse) (*CursorRateLimitData, error) {
	if body == nil || body.PlanUsage == nil {
		return &CursorRateLimitData{}, nil
	}

	cycleEndMs := parseCursorMillis(body.BillingCycleEnd)
	cycleStartMs := parseCursorMillis(body.BillingCycleStart)
	if cycleEndMs <= 0 {
		return &CursorRateLimitData{}, nil
	}

	resetAtSec := cycleEndMs / 1000
	windowDurationMinutes := 0
	if cycleStartMs > 0 && cycleEndMs > cycleStartMs {
		windowDurationMinutes = int(math.Round(float64(cycleEndMs-cycleStartMs) / 60_000.0))
	}

	specs := []struct {
		id    string
		name  string
		value *float64
	}{
		{id: "totalPercentUsed", name: "Overall", value: body.PlanUsage.TotalPercentUsed},
		{id: "autoPercentUsed", name: "Cursor Models", value: body.PlanUsage.AutoPercentUsed},
		{id: "apiPercentUsed", name: "Other Models", value: body.PlanUsage.APIPercentUsed},
	}

	windows := make([]CursorRateLimitWindow, 0, len(specs))
	for _, spec := range specs {
		if spec.value == nil || math.IsNaN(*spec.value) || math.IsInf(*spec.value, 0) {
			continue
		}
		pct := *spec.value
		if pct < 0 {
			pct = 0
		} else if pct > 100 {
			pct = 100
		}
		windows = append(windows, CursorRateLimitWindow{
			LimitID:               spec.id,
			LimitName:             spec.name,
			UsagePercentage:       pct,
			ResetAt:               resetAtSec,
			WindowDurationMinutes: windowDurationMinutes,
		})
	}

	return &CursorRateLimitData{
		Windows: windows,
	}, nil
}

func parseCursorMillis(v any) int64 {
	switch t := v.(type) {
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0
		}
		return int64(t)
	case int64:
		return t
	case string:
		trimmed := strings.TrimSpace(t)
		if trimmed == "" {
			return 0
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0
		}
		return int64(f)
	default:
		return 0
	}
}

// CursorSyncSkipReason classifies expected non-error skip reasons for Cursor usage sync.
func CursorSyncSkipReason(err error) (string, bool) {
	switch {
	case errors.Is(err, model.ErrCursorAuthMissing):
		return "missing_cursor_auth", true
	case errors.Is(err, model.ErrCursorTokenInvalid):
		return "invalid_cursor_token", true
	default:
		return "", false
	}
}
