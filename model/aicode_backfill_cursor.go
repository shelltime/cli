package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	cursorUsagePageSize    = 1000
	cursorDefaultLookback  = 90 * 24 * time.Hour
	cursorMaxResponseBytes = 32 << 20
)

type cursorUsageEventRaw struct {
	Timestamp      any               `json:"timestamp"`
	Model          string            `json:"model"`
	ConversationID string            `json:"conversationId"`
	TokenUsage     *cursorTokenUsage `json:"tokenUsage"`
}

type cursorTokenUsage struct {
	InputTokens      *float64 `json:"inputTokens"`
	OutputTokens     *float64 `json:"outputTokens"`
	CacheReadTokens  *float64 `json:"cacheReadTokens"`
	CacheWriteTokens *float64 `json:"cacheWriteTokens"`
	TotalCents       *float64 `json:"totalCents"`
}

type cursorNormalizedRecord struct {
	timestampMs         int64
	model               string
	sessionID           string
	inputTokens         int
	outputTokens        int
	cacheReadTokens     int
	cacheCreationTokens int
	reportedCostUSD     *float64
	identityHash        string
	dedupeKey           string
}

// FetchCursorUsageSessions fetches billed token usage from Cursor's DashboardService
// (personal scope plus every team the account belongs to) and groups them into BackfillSessions.
func FetchCursorUsageSessions(ctx context.Context, auth *CursorAuthData, opts BackfillOptions) (*BackfillParseResult, error) {
	return fetchCursorUsageSessionsWithClient(ctx, auth, opts, nil)
}

func fetchCursorUsageSessionsWithClient(ctx context.Context, auth *CursorAuthData, opts BackfillOptions, client *http.Client) (*BackfillParseResult, error) {
	if auth == nil || strings.TrimSpace(auth.AccessToken) == "" {
		return nil, ErrCursorAuthMissing
	}
	if client == nil {
		client = cursorNoRedirectClient(15 * time.Second)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(auth.Endpoint), "/")
	if endpoint == "" {
		endpoint = DefaultCursorAPIEndpoint
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	since := opts.Since
	if since.IsZero() {
		since = now.Add(-cursorDefaultLookback)
	}
	until := opts.Until
	if until.IsZero() || until.After(now) {
		until = now
	}
	sinceMs := since.UnixMilli()
	untilMs := until.UnixMilli()
	if sinceMs < 0 {
		sinceMs = 0
	}
	if sinceMs > untilMs {
		return &BackfillParseResult{}, nil
	}

	accountKey, err := cursorAccountKeyFromJWT(auth.AccessToken)
	if err != nil {
		return nil, err
	}

	teamIDs, userID, err := fetchCursorMembership(ctx, endpoint, auth.AccessToken, client)
	if err != nil {
		return nil, err
	}

	// Read personal scope (teamID == nil) plus each team scope.
	scopes := make([]*int64, 0, 1+len(teamIDs))
	scopes = append(scopes, nil)
	for i := range teamIDs {
		id := teamIDs[i]
		scopes = append(scopes, &id)
	}

	var allRecords []cursorNormalizedRecord
	for _, teamID := range scopes {
		recs, err := fetchCursorScopeUsage(ctx, endpoint, auth.AccessToken, accountKey, sinceMs, untilMs, teamID, userID, client)
		if err != nil {
			return nil, err
		}
		allRecords = append(allRecords, recs...)
	}

	machine := currentBackfillMachine()
	bySession := make(map[string]*BackfillSession)
	for _, rec := range allRecords {
		ts := time.UnixMilli(rec.timestampMs).UTC()
		sessionID := rec.sessionID
		if sessionID == "" {
			sessionID = "cursor-day-" + ts.Format("2006-01-02")
		}

		ev := newBackfillEvent(AICodeClientCursor, AICodeEventApiRequest, rec.dedupeKey, sessionID, ts)
		machine.apply(&ev)
		ev.Model = rec.model
		inTok := rec.inputTokens
		outTok := rec.outputTokens
		cacheRead := rec.cacheReadTokens
		cacheWrite := rec.cacheCreationTokens
		ev.InputTokens = &inTok
		ev.OutputTokens = &outTok
		ev.CacheReadTokens = &cacheRead
		ev.CacheCreationTokens = &cacheWrite
		if rec.reportedCostUSD != nil {
			cost := *rec.reportedCostUSD
			ev.CostUSD = &cost
		}

		sess, ok := bySession[sessionID]
		if !ok {
			sess = &BackfillSession{
				ID:         sessionID,
				ClientType: AICodeClientCursor,
			}
			bySession[sessionID] = sess
		}
		sess.Events = append(sess.Events, ev)
	}

	sessions := make([]*BackfillSession, 0, len(bySession))
	for _, s := range bySession {
		finalizeBackfillSession(s)
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		if !sessions[i].Start.Equal(sessions[j].Start) {
			return sessions[i].Start.Before(sessions[j].Start)
		}
		return sessions[i].ID < sessions[j].ID
	})

	return &BackfillParseResult{
		Sessions: sessions,
		Files:    len(scopes),
	}, nil
}

func cursorAccountKeyFromJWT(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		// Allow non-JWT opaque tokens by hashing the token itself as account key.
		sum := sha256.Sum256([]byte(token))
		return hex.EncodeToString(sum[:]), nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		sum := sha256.Sum256([]byte(token))
		return hex.EncodeToString(sum[:]), nil
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return "", ErrCursorTokenInvalid
	}
	sub, ok := claims["sub"].(string)
	if !ok || strings.TrimSpace(sub) == "" {
		return "", ErrCursorTokenInvalid
	}
	sum := sha256.Sum256([]byte(sub))
	return hex.EncodeToString(sum[:]), nil
}

func cursorDashboardPost(ctx context.Context, endpoint, method, accessToken string, body []byte, client *http.Client) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/aiserver.v1.DashboardService/"+method, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("connect-protocol-version", "1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "shelltime-cli")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrCursorTokenInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cursor %s returned status %d", method, resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, cursorMaxResponseBytes))
	if err != nil {
		return nil, err
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("cursor %s invalid JSON: %w", method, err)
	}
	if parsed == nil {
		return nil, fmt.Errorf("cursor %s returned null body", method)
	}
	if _, hasErr := parsed["error"]; hasErr {
		return nil, fmt.Errorf("cursor %s returned error response", method)
	}
	if _, hasMsg := parsed["message"]; hasMsg {
		return nil, fmt.Errorf("cursor %s returned error response", method)
	}
	if _, hasCode := parsed["code"]; hasCode {
		return nil, fmt.Errorf("cursor %s returned error response", method)
	}
	return parsed, nil
}

func fetchCursorMembership(ctx context.Context, endpoint, accessToken string, client *http.Client) ([]int64, int64, error) {
	teamsBody, err := cursorDashboardPost(ctx, endpoint, "GetTeams", accessToken, []byte("{}"), client)
	if err != nil {
		return nil, 0, err
	}
	meBody, err := cursorDashboardPost(ctx, endpoint, "GetMe", accessToken, []byte("{}"), client)
	if err != nil {
		return nil, 0, err
	}

	userIDFloat, ok := meBody["userId"].(float64)
	if !ok || userIDFloat <= 0 || math.Trunc(userIDFloat) != userIDFloat {
		return nil, 0, fmt.Errorf("cursor GetMe returned invalid userId")
	}
	userID := int64(userIDFloat)

	seenTeams := map[int64]bool{}
	var teamIDs []int64
	if rawTeams, ok := teamsBody["teams"].([]any); ok {
		for _, entry := range rawTeams {
			m, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			idFloat, ok := m["id"].(float64)
			if !ok || idFloat <= 0 || math.Trunc(idFloat) != idFloat {
				return nil, 0, fmt.Errorf("cursor GetTeams returned invalid team id")
			}
			id := int64(idFloat)
			if !seenTeams[id] {
				seenTeams[id] = true
				teamIDs = append(teamIDs, id)
			}
		}
	}
	sort.Slice(teamIDs, func(i, j int) bool { return teamIDs[i] < teamIDs[j] })
	return teamIDs, userID, nil
}

func fetchCursorScopeUsage(
	ctx context.Context,
	endpoint, accessToken, accountKey string,
	sinceMs, untilMs int64,
	teamID *int64,
	userID int64,
	client *http.Client,
) ([]cursorNormalizedRecord, error) {
	type pageSlice struct {
		start   int
		end     int
		overlap int
	}

	var (
		total          *int
		normalized     []*cursorNormalizedRecord
		pages          []pageSlice
		previousEvents []any
	)

	for page := 1; ; page++ {
		payload := map[string]any{
			"page":      page,
			"pageSize":  cursorUsagePageSize,
			"startDate": strconv.FormatInt(sinceMs, 10),
			"endDate":   strconv.FormatInt(untilMs, 10),
		}
		if teamID != nil {
			payload["teamId"] = *teamID
			payload["userId"] = userID
		}
		reqBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		body, err := cursorDashboardPost(ctx, endpoint, "GetFilteredUsageEvents", accessToken, reqBytes, client)
		if err != nil {
			return nil, err
		}

		var events []any
		if len(body) == 0 {
			z := 0
			total = &z
		} else {
			if rawCount, exists := body["totalUsageEventsCount"]; exists {
				cFloat, ok := rawCount.(float64)
				if !ok || cFloat < 0 || math.Trunc(cFloat) != cFloat {
					return nil, fmt.Errorf("cursor GetFilteredUsageEvents returned invalid totalUsageEventsCount")
				}
				c := int(cFloat)
				if total != nil && *total != c {
					return nil, fmt.Errorf("cursor GetFilteredUsageEvents totalUsageEventsCount changed mid-pagination")
				}
				total = &c
			}
			if rawEvents, exists := body["usageEventsDisplay"]; exists {
				arr, ok := rawEvents.([]any)
				if !ok || len(arr) > cursorUsagePageSize {
					return nil, fmt.Errorf("cursor GetFilteredUsageEvents returned invalid usageEventsDisplay")
				}
				events = arr
			} else if !(len(body) == 1 && total != nil) {
				return nil, fmt.Errorf("cursor GetFilteredUsageEvents missing usageEventsDisplay")
			}
		}

		start := len(normalized)
		overlap := 0
		if len(events) > 0 && len(previousEvents) > 0 {
			if cursorPagesMayOverlap(previousEvents, events) {
				overlap = cursorBoundaryOverlap(cursorCanonicalList(previousEvents), cursorCanonicalList(events))
			}
		}
		previousEvents = events

		for _, raw := range events {
			rec, err := normalizeCursorUsageEvent(raw, sinceMs, untilMs, teamID)
			if err != nil {
				slog.Debug("Skipping malformed Cursor usage event", slog.Any("err", err))
				normalized = append(normalized, nil)
				continue
			}
			normalized = append(normalized, rec)
		}
		pages = append(pages, pageSlice{start: start, end: len(normalized), overlap: overlap})

		if len(events) < cursorUsagePageSize {
			break
		}
	}

	rawCount := len(normalized)
	if total != nil && rawCount < *total {
		return nil, fmt.Errorf("cursor GetFilteredUsageEvents returned incomplete pages (%d < %d)", rawCount, *total)
	}
	removalsRemaining := 0
	if total != nil {
		removalsRemaining = rawCount - *total
	}

	occurrences := map[string]int{}
	records := make([]cursorNormalizedRecord, 0, rawCount)
	for _, p := range pages {
		removalCount := min(removalsRemaining, p.overlap)
		removalsRemaining -= removalCount
		for idx := p.start; idx < p.end; idx++ {
			rec := normalized[idx]
			if idx < p.start+removalCount || rec == nil {
				continue
			}
			occ := occurrences[rec.identityHash]
			occurrences[rec.identityHash] = occ + 1
			rec.dedupeKey = fmt.Sprintf("cursor-account:%s:%s:%d", accountKey, rec.identityHash, occ)
			records = append(records, *rec)
		}
	}
	if removalsRemaining != 0 {
		return nil, fmt.Errorf("cursor GetFilteredUsageEvents inconsistent page boundaries (remaining=%d)", removalsRemaining)
	}

	return records, nil
}

func normalizeCursorUsageEvent(raw any, sinceMs, untilMs int64, teamID *int64) (*cursorNormalizedRecord, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid cursor usage event object")
	}
	tuRaw, hasUsage := m["tokenUsage"]
	if !hasUsage || tuRaw == nil {
		return nil, nil
	}
	tu, ok := tuRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid cursor tokenUsage object")
	}

	parseNonNeg := func(key string) (float64, bool, error) {
		v, ok := tu[key]
		if !ok || v == nil {
			return 0, false, nil
		}
		f, ok := v.(float64)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			return 0, false, fmt.Errorf("invalid cursor tokenUsage.%s", key)
		}
		return f, true, nil
	}

	inF, _, err := parseNonNeg("inputTokens")
	if err != nil {
		return nil, err
	}
	outF, _, err := parseNonNeg("outputTokens")
	if err != nil {
		return nil, err
	}
	cacheReadF, _, err := parseNonNeg("cacheReadTokens")
	if err != nil {
		return nil, err
	}
	cacheWriteF, _, err := parseNonNeg("cacheWriteTokens")
	if err != nil {
		return nil, err
	}
	totalCents, hasCents, err := parseNonNeg("totalCents")
	if err != nil {
		return nil, err
	}

	tsMs, err := parseCursorTimestampMs(m["timestamp"])
	if err != nil {
		return nil, err
	}
	modelName, _ := m["model"].(string)
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("invalid cursor usage event: missing model")
	}
	if tsMs < sinceMs || tsMs > untilMs {
		return nil, nil
	}

	sessionID, _ := m["conversationId"].(string)
	inTok := int(math.Trunc(inF))
	outTok := int(math.Trunc(outF))
	cacheRead := int(math.Trunc(cacheReadF))
	cacheWrite := int(math.Trunc(cacheWriteF))

	var costUSD *float64
	if hasCents {
		c := totalCents / 100.0
		costUSD = &c
	}

	identityFields := make([]any, 0, 7)
	if teamID != nil {
		identityFields = append(identityFields, *teamID)
	}
	identityFields = append(identityFields,
		tsMs,
		modelName,
		sessionID,
		map[string]int{
			"uncachedInputTokens": inTok,
			"cachedInputTokens":   cacheRead,
			"cacheCreationTokens": cacheWrite,
			"outputTokens":        outTok,
			"reasoningTokens":     0,
		},
		costUSD,
	)
	identityJSON, _ := json.Marshal(identityFields)
	sum := sha256.Sum256(identityJSON)

	return &cursorNormalizedRecord{
		timestampMs:         tsMs,
		model:               modelName,
		sessionID:           sessionID,
		inputTokens:         inTok,
		outputTokens:        outTok,
		cacheReadTokens:     cacheRead,
		cacheCreationTokens: cacheWrite,
		reportedCostUSD:     costUSD,
		identityHash:        hex.EncodeToString(sum[:]),
	}, nil
}

func parseCursorTimestampMs(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, fmt.Errorf("invalid cursor event timestamp")
		}
		return int64(t), nil
	case string:
		trimmed := strings.TrimSpace(t)
		if trimmed == "" {
			return 0, fmt.Errorf("empty cursor event timestamp")
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, fmt.Errorf("invalid cursor event timestamp %q", t)
		}
		return int64(f), nil
	default:
		return 0, fmt.Errorf("missing cursor event timestamp")
	}
}

func cursorPagesMayOverlap(prev, curr []any) bool {
	firstMap, ok := curr[0].(map[string]any)
	if !ok {
		return true
	}
	firstTs, err := parseCursorTimestampMs(firstMap["timestamp"])
	if err != nil {
		return true
	}
	for _, ev := range prev {
		if m, ok := ev.(map[string]any); ok {
			if ts, err := parseCursorTimestampMs(m["timestamp"]); err == nil && ts == firstTs {
				return true
			}
		}
	}
	return false
}

func cursorCanonicalList(events []any) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = cursorCanonicalJSON(ev)
	}
	return out
}

func cursorCanonicalJSON(v any) string {
	switch x := v.(type) {
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(cursorCanonicalJSON(item))
		}
		b.WriteByte(']')
		return b.String()
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			b.Write(kb)
			b.WriteByte(':')
			b.WriteString(cursorCanonicalJSON(x[k]))
		}
		b.WriteByte('}')
		return b.String()
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "null"
		}
		return string(b)
	}
}

// cursorBoundaryOverlap computes the longest suffix of previous that matches a prefix of current via KMP.
func cursorBoundaryOverlap(previous, current []string) int {
	if len(previous) == 0 || len(current) == 0 {
		return 0
	}
	seq := make([]string, 0, len(current)+1+len(previous))
	seq = append(seq, current...)
	seq = append(seq, "\x00")
	seq = append(seq, previous...)
	pi := make([]int, len(seq))
	for i := 1; i < len(seq); i++ {
		j := pi[i-1]
		for j > 0 && seq[i] != seq[j] {
			j = pi[j-1]
		}
		if seq[i] == seq[j] {
			j++
		}
		pi[i] = j
	}
	return pi[len(pi)-1]
}
