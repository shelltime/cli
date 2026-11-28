package model

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

// Offline fallback for Antigravity records without explicit model names.
// Cross-checked against t3code's antigravityUsageReader.ts.
var antigravityModelIDs = map[int]string{
	246:  "gemini-2.5-pro",
	312:  "gemini-2.5-flash",
	313:  "gemini-2.5-flash-thinking",
	329:  "gemini-2.5-flash-thinking",
	330:  "gemini-2.5-flash-lite",
	281:  "claude-sonnet-4",
	282:  "claude-sonnet-4",
	290:  "claude-opus-4",
	291:  "claude-opus-4",
	333:  "claude-sonnet-4-5",
	334:  "claude-sonnet-4-5",
	340:  "claude-haiku-4-5",
	341:  "claude-haiku-4-5",
	342:  "gpt-oss-120b-medium",
	1026: "claude-opus-4-6",
	1035: "claude-sonnet-4-6",
	1016: "gemini-3.1-pro",
	1036: "gemini-3.1-pro",
	1037: "gemini-3.1-pro",
	1018: "gemini-3-flash-preview",
	1084: "gemini-3-flash-preview",
	1047: "gemini-3-flash-preview",
	1020: "gemini-3.5-flash-medium",
	1021: "gemini-3.1-flash-image",
	1050: "gemini-3.1-flash-lite",
	1071: "gemini-3.6-flash-high",
	1072: "gemini-3.6-flash-medium",
	1073: "gemini-3.6-flash-low",
	1132: "gemini-3.5-flash-high",
	1133: "gemini-3.5-flash-high",
	1187: "gemini-3.5-flash-extra-low",
	1196: "gemini-3.6-flash",
	1266: "gemini-3.6-flash",
	1298: "gemini-3.7-flash-high",
	1299: "gemini-3.7-flash-medium",
	1300: "gemini-3.7-flash-low",
	1301: "gemini-3.7-flash-tiered",
	1318: "gemini-3.8-flash-high",
	1319: "gemini-3.8-flash-medium",
	1320: "gemini-3.8-flash-low",
	1322: "gemini-3.8-flash-tiered",
	1402: "claude-opus-5-5",
	1417: "gemini-4-argon",
	1418: "gemini-4-argon",
	1419: "gemini-4-argon",
	1427: "gemini-4-carbon",
	1437: "gemini-4-carbon",
	1525: "gemini-4-hydrogen",
	1526: "gemini-4-hydrogen",
	1527: "gemini-4-hydrogen",
	1532: "gemini-4-neon",
	1533: "gemini-4-neon",
	1537: "gemini-pro-orchestrator-neon",
	1667: "gemini-pro-orchestrator",
	1669: "gemini-4-hydrogen-orchestrator",
}

var (
	agyParenSuffixRe     = regexp.MustCompile(`\s*\(([^)]*)\)\s*$`)
	agySpaceRe           = regexp.MustCompile(`\s+`)
	agyClaudeVersionRe   = regexp.MustCompile(`^claude-(\d+(?:\.\d+)?)-(sonnet|opus|haiku)`)
	agyConcreteModelRe   = regexp.MustCompile(`^(?:gemini-\d|claude-(?:sonnet|opus|haiku)-\d|gpt-)`)
	agyFlashAgentAliasRe = regexp.MustCompile(`^gemini-3-flash-(?:agent(?:-[ab])?|[ab])$`)
	agyModelPrefixRe     = regexp.MustCompile(`^model[_-]`)
	agyRateSuffixRe      = regexp.MustCompile(`-(?:extra-low|low|medium|high|tiered|control|thinking|agent|orchestrator|trial|n|r)$`)
)

// AntigravityDataRoots returns the directories holding Antigravity conversation SQLite DBs:
// ANTIGRAVITY_DATA_DIR (comma-separated) if set, otherwise the default ~/.gemini/antigravity*
// and ~/.config/antigravity directories. If a root contains a conversations/ subfolder, that
// subfolder is returned.
func AntigravityDataRoots() []string {
	home, _ := os.UserHomeDir()
	var configured []string
	if env := strings.TrimSpace(os.Getenv("ANTIGRAVITY_DATA_DIR")); env != "" {
		configured = splitPathList(env)
	}
	var rawRoots []string
	if len(configured) > 0 {
		for _, r := range configured {
			rawRoots = append(rawRoots, expandAntigravityHome(r, home))
		}
	} else if home != "" {
		for _, name := range []string{"antigravity", "antigravity-cli", "antigravity-ide", "antigravity-backup"} {
			rawRoots = append(rawRoots, filepath.Join(home, ".gemini", name))
		}
		rawRoots = append(rawRoots, filepath.Join(home, ".config", "antigravity"))
	}

	seen := map[string]bool{}
	roots := make([]string, 0, len(rawRoots))
	for _, root := range rawRoots {
		if root == "" {
			continue
		}
		resolved := root
		if r, err := filepath.EvalSymlinks(root); err == nil {
			resolved = r
		}
		nested := filepath.Join(resolved, "conversations")
		target := resolved
		if info, err := os.Stat(nested); err == nil && info.IsDir() {
			target = nested
		}
		if r, err := filepath.EvalSymlinks(target); err == nil {
			target = r
		}
		if !seen[target] {
			seen[target] = true
			roots = append(roots, target)
		}
	}
	return roots
}

func expandAntigravityHome(p, home string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" && home != "" {
		return home
	}
	if strings.HasPrefix(p, "~/") && home != "" {
		return filepath.Join(home, p[2:])
	}
	return p
}

// AntigravityDBFingerprint returns a fingerprint covering both the .db file and its optional -wal file.
func AntigravityDBFingerprint(path string) (string, time.Time, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", time.Time{}, err
	}
	latestMod := st.ModTime()
	part := fmt.Sprintf("%d:%d", st.Size(), st.ModTime().UnixNano())
	walPart := "-"
	if wst, err := os.Stat(path + "-wal"); err == nil {
		walPart = fmt.Sprintf("%d:%d", wst.Size(), wst.ModTime().UnixNano())
		if wst.ModTime().After(latestMod) {
			latestMod = wst.ModTime()
		}
	}
	return part + "/" + walPart, latestMod, nil
}

// CollectAntigravityDBFiles walks roots and returns sorted canonical *.db paths (ignoring symlinks).
// Files whose .db and -wal mtime are both more than 24h before since are pruned.
func CollectAntigravityDBFiles(roots []string, since time.Time) ([]string, error) {
	seenRoots := map[string]bool{}
	seenFiles := map[string]bool{}
	var files []string

	var cutoff time.Time
	if !since.IsZero() {
		cutoff = since.Add(-24 * time.Hour)
	}

	for _, root := range roots {
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			continue
		}
		if seenRoots[resolved] {
			continue
		}
		seenRoots[resolved] = true

		_ = filepath.WalkDir(resolved, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// Ignore symlinks (both files and directories), matching t3code.
			if d.Type()&os.ModeSymlink != 0 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".db") {
				return nil
			}
			canonical, err := filepath.EvalSymlinks(path)
			if err != nil {
				canonical = path
			}
			if seenFiles[canonical] {
				return nil
			}
			if !cutoff.IsZero() {
				_, modTime, err := AntigravityDBFingerprint(canonical)
				if err == nil && modTime.Before(cutoff) {
					return nil
				}
			}
			seenFiles[canonical] = true
			files = append(files, canonical)
			return nil
		})
	}
	sort.Strings(files)
	return files, nil
}

// ParseAntigravityDBs scans Antigravity conversation SQLite databases under roots and
// rebuilds sessions with deduplicated token usage events.
func ParseAntigravityDBs(roots []string, opts BackfillOptions) (*BackfillParseResult, error) {
	files, err := CollectAntigravityDBFiles(roots, opts.Since)
	if err != nil {
		return nil, err
	}
	return ParseAntigravityDBFiles(files, opts)
}

// ParseAntigravityDBFiles parses a specific list of Antigravity SQLite *.db files into BackfillSessions.
func ParseAntigravityDBFiles(files []string, opts BackfillOptions) (*BackfillParseResult, error) {
	type dbResult struct {
		candidates []agyUsageCandidate
		models     map[int]agyLearnedModel
	}

	learnedModels := map[int]agyLearnedModel{}
	var pending []dbResult
	badItems := 0
	var failedFiles []string

	for _, path := range files {
		st, err := os.Stat(path)
		if err != nil {
			badItems++
			failedFiles = append(failedFiles, path)
			continue
		}
		sessionID := strings.TrimSuffix(filepath.Base(path), ".db")
		cands, models, rowBad, err := readAntigravityDatabase(path, sessionID, st.ModTime().UnixMilli())
		badItems += rowBad
		if err != nil {
			badItems++
			failedFiles = append(failedFiles, path)
			continue
		}
		if cands == nil && models == nil {
			// Not a conversation DB (e.g. conversation_summaries.db)
			continue
		}
		for id, lm := range models {
			rememberAgyModel(learnedModels, id, lm)
		}
		pending = append(pending, dbResult{candidates: cands, models: models})
	}

	// Union-find across all candidates from all databases (matches t3code's cross-DB dedup).
	uf := newAgyUnionFind()
	for _, item := range pending {
		for _, cand := range item.candidates {
			lm, hasLearned := learnedModels[cand.modelID]
			learnedName := ""
			if hasLearned && !lm.ambiguous {
				learnedName = lm.name
			}
			fallback := ""
			if !(hasLearned && lm.ambiguous) {
				fallback = antigravityModelIDs[cand.modelID]
			}

			modelName := ""
			modelQuality := cand.modelQuality
			switch {
			case cand.modelQuality == 4 && cand.model != "":
				modelName = cand.model
			case learnedName != "":
				modelName = learnedName
				modelQuality = 3
			case fallback != "":
				modelName = fallback
				modelQuality = 2
			case cand.model != "":
				modelName = cand.model
			case cand.modelID > 0:
				modelName = fmt.Sprintf("antigravity-model-%d", cand.modelID)
			default:
				modelName = "antigravity-unknown"
			}

			cand.model = modelName
			cand.modelQuality = modelQuality
			uf.append(cand)
		}
	}

	machine := currentBackfillMachine()
	bySession := map[string]*BackfillSession{}
	for idx, g := range uf.groups {
		if g.parent != idx {
			continue
		}
		ts := time.UnixMilli(g.timestampMs).UTC()
		ev := newBackfillEvent(AICodeClientAntigravity, AICodeEventApiRequest, g.dedupeKey, g.sessionID, ts)
		machine.apply(&ev)
		ev.Model = g.model
		inTok := g.uncachedInputTokens
		outTok := g.outputTokens
		cacheRead := g.cachedInputTokens
		cacheWrite := g.cacheCreationTokens
		reasoning := g.reasoningTokens
		ev.InputTokens = &inTok
		ev.OutputTokens = &outTok
		ev.CacheReadTokens = &cacheRead
		ev.CacheCreationTokens = &cacheWrite
		if reasoning > 0 {
			ev.ReasoningTokens = &reasoning
		}

		sess, ok := bySession[g.sessionID]
		if !ok {
			sess = &BackfillSession{
				ID:         g.sessionID,
				ClientType: AICodeClientAntigravity,
			}
			bySession[g.sessionID] = sess
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
		Sessions:    sessions,
		Files:       len(files),
		BadLines:    badItems,
		FailedFiles: failedFiles,
	}, nil
}

// --- Protobuf wire decoder (matching t3code's antigravityUsageReader.ts) ---

type pbFieldValue struct {
	num   uint64
	bytes []byte
	isNum bool
}

type pbFields map[int][]pbFieldValue

func parseProtobufFields(data []byte) (pbFields, error) {
	result := make(pbFields)
	offset := 0
	readVarint := func() (uint64, error) {
		var val uint64
		for shift := uint(0); shift < 70; shift += 7 {
			if offset >= len(data) {
				return 0, errors.New("truncated protobuf varint")
			}
			b := data[offset]
			offset++
			if shift == 63 && b > 1 {
				return 0, errors.New("overflow in protobuf varint")
			}
			val |= uint64(b&0x7f) << shift
			if b < 0x80 {
				return val, nil
			}
		}
		return 0, errors.New("invalid protobuf varint")
	}

	for offset < len(data) {
		tag, err := readVarint()
		if err != nil {
			return nil, err
		}
		fieldNum := int(tag >> 3)
		wireType := int(tag & 7)
		if fieldNum == 0 {
			return nil, errors.New("invalid protobuf field number 0")
		}
		switch wireType {
		case 0:
			v, err := readVarint()
			if err != nil {
				return nil, err
			}
			result[fieldNum] = append(result[fieldNum], pbFieldValue{num: v, isNum: true})
		case 1:
			if offset+8 > len(data) {
				return nil, errors.New("truncated protobuf fixed64")
			}
			offset += 8
		case 5:
			if offset+4 > len(data) {
				return nil, errors.New("truncated protobuf fixed32")
			}
			offset += 4
		case 2:
			length, err := readVarint()
			if err != nil {
				return nil, err
			}
			if length > uint64(len(data)-offset) {
				return nil, errors.New("truncated protobuf bytes field")
			}
			slice := data[offset : offset+int(length)]
			offset += int(length)
			result[fieldNum] = append(result[fieldNum], pbFieldValue{bytes: slice})
		default:
			return nil, fmt.Errorf("unsupported protobuf wire type %d", wireType)
		}
	}
	return result, nil
}

func pbNumberAt(f pbFields, key int) int {
	entries := f[key]
	if len(entries) == 0 || !entries[0].isNum {
		return 0
	}
	return int(entries[0].num)
}

func pbBytesAt(f pbFields, key int) []byte {
	entries := f[key]
	if len(entries) == 0 || entries[0].isNum {
		return nil
	}
	return entries[0].bytes
}

func pbNested(f pbFields, key int) pbFields {
	b := pbBytesAt(f, key)
	if len(b) == 0 {
		return nil
	}
	sub, err := parseProtobufFields(b)
	if err != nil {
		return nil
	}
	return sub
}

func pbTextAt(f pbFields, key int) string {
	b := pbBytesAt(f, key)
	if len(b) == 0 || !utf8.Valid(b) {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func pbTimestampMs(f pbFields) *int64 {
	if f == nil {
		return nil
	}
	sec := int64(pbNumberAt(f, 1))
	if sec <= 0 {
		return nil
	}
	nanos := int64(pbNumberAt(f, 2))
	ms := sec*1000 + nanos/1_000_000
	return &ms
}

// --- Model name normalization & learning ---

type agyLearnedModel struct {
	name      string
	ambiguous bool
}

func normalizeAntigravityModelName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	lower = agyParenSuffixRe.ReplaceAllString(lower, "-$1")
	lower = agySpaceRe.ReplaceAllString(lower, "-")
	if strings.HasPrefix(lower, "claude-") {
		lower = agyClaudeVersionRe.ReplaceAllString(lower, "claude-$2-$1")
		lower = strings.ReplaceAll(lower, ".", "-")
	}
	return lower
}

func isConcreteAntigravityModel(model string) bool {
	return agyConcreteModelRe.MatchString(model) && !agyFlashAgentAliasRe.MatchString(model)
}

func antigravityRateModel(model string) string {
	base := agyRateSuffixRe.ReplaceAllString(model, "")
	switch {
	case agyFlashAgentAliasRe.MatchString(model):
		return "gemini-3.5-flash"
	case model == "gemini-3.7-flash-safety-le":
		return "gemini-3.7-flash"
	case model == "gemini-pro-orchestrator-neon":
		return "gemini-4-neon"
	case model == "gemini-pro-orchestrator-g1-pro":
		return "gemini-4-argon"
	case model == "gemini-pro-orchestrator":
		return "gemini-4-carbon"
	case base == "gemini-3-flash":
		return "gemini-3-flash-preview"
	default:
		return base
	}
}

func rememberAgyModel(models map[int]agyLearnedModel, id int, lm agyLearnedModel) {
	prev, exists := models[id]
	if !exists {
		models[id] = lm
		return
	}
	if prev.ambiguous || lm.ambiguous {
		models[id] = agyLearnedModel{ambiguous: true}
		return
	}
	if prev.name != lm.name && antigravityRateModel(prev.name) != antigravityRateModel(lm.name) {
		models[id] = agyLearnedModel{ambiguous: true}
	}
}

// --- SQLite reading & candidate extraction ---

type agyMetadata struct {
	model       string
	modelID     int
	timestampMs *int64
	usages      []pbFields
}

type agyIndexedMetadata struct {
	idx   int
	entry agyMetadata
}

func parseAgyMetadata(raw []byte, isStep bool) (agyMetadata, error) {
	root, err := parseProtobufFields(raw)
	if err != nil {
		return agyMetadata{}, err
	}
	var data pbFields
	if isStep {
		data = root
	} else {
		if pbBytesAt(root, 1) == nil {
			return agyMetadata{}, errors.New("missing Antigravity generation metadata")
		}
		data = pbNested(root, 1)
	}

	modelMsg := data
	if isStep {
		modelMsg = pbNested(data, 24)
	}

	usageFieldNum := 4
	retryFieldNum := 17
	if isStep {
		usageFieldNum = 9
		retryFieldNum = 28
	}

	var usages []pbFields
	var primaryUsage pbFields
	if uBytes := pbBytesAt(data, usageFieldNum); len(uBytes) > 0 {
		copied := append([]byte(nil), uBytes...)
		if parsed, err := parseProtobufFields(copied); err == nil {
			primaryUsage = parsed
			usages = append(usages, parsed)
		}
	}

	for _, retryVal := range data[retryFieldNum] {
		if retryVal.isNum || len(retryVal.bytes) == 0 {
			continue
		}
		retryMsg, err := parseProtobufFields(retryVal.bytes)
		if err != nil {
			continue
		}
		if ruBytes := pbBytesAt(retryMsg, 2); len(ruBytes) > 0 {
			copied := append([]byte(nil), ruBytes...)
			if parsed, err := parseProtobufFields(copied); err == nil {
				usages = append(usages, parsed)
			}
		}
	}

	var nameA, nameB string
	var modelID int
	if isStep {
		nameA = normalizeAntigravityModelName(pbTextAt(modelMsg, 12))
		nameB = normalizeAntigravityModelName(pbTextAt(modelMsg, 8))
		modelID = pbNumberAt(modelMsg, 1)
	} else {
		nameA = normalizeAntigravityModelName(pbTextAt(modelMsg, 19))
		nameB = normalizeAntigravityModelName(pbTextAt(modelMsg, 21))
		modelID = pbNumberAt(modelMsg, 3)
	}
	if modelID == 0 && primaryUsage != nil {
		modelID = pbNumberAt(primaryUsage, 1)
	}

	chosenModel := ""
	for _, n := range []string{nameA, nameB} {
		if isConcreteAntigravityModel(n) {
			chosenModel = n
			break
		}
	}
	if chosenModel == "" {
		for _, n := range []string{nameA, nameB} {
			if n != "" && !agyModelPrefixRe.MatchString(n) {
				chosenModel = n
				break
			}
		}
	}

	var tsMs *int64
	if isStep {
		tsMs = pbTimestampMs(pbNested(data, 8))
		if tsMs == nil {
			tsMs = pbTimestampMs(pbNested(data, 1))
		}
	} else {
		tsMs = pbTimestampMs(pbNested(pbNested(data, 9), 4))
	}

	return agyMetadata{
		model:       chosenModel,
		modelID:     modelID,
		timestampMs: tsMs,
		usages:      usages,
	}, nil
}

func readAntigravityDatabase(path, sessionID string, fallbackTimestampMs int64) ([]agyUsageCandidate, map[int]agyLearnedModel, int, error) {
	// Open strictly read-only with URI-encoded path so paths containing ?, #, or % work safely
	// and SQLite never creates -wal/-shm files next to a live agent DB.
	dsnURL := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "mode=ro&_pragma=busy_timeout(100)&_pragma=query_only(ON)",
	}
	db, err := sql.Open("sqlite", dsnURL.String())
	if err != nil {
		return nil, nil, 0, err
	}
	defer db.Close()

	tables := map[string]bool{}
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		return nil, nil, 0, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tables[name] = true
		}
	}
	_ = rows.Close()

	if !tables["gen_metadata"] && !tables["steps"] {
		return nil, nil, 0, nil
	}

	badRows := 0
	readMeta := func(query string, pageRows int, isStep bool) ([]agyIndexedMetadata, error) {
		var entries []agyIndexedMetadata
		var after int64 = -1
		hasAfter := false
		for {
			var qRows *sql.Rows
			var qErr error
			if !hasAfter {
				qRows, qErr = db.Query(query, -9223372036854775808, pageRows)
			} else {
				qRows, qErr = db.Query(query, after, pageRows)
			}
			if qErr != nil {
				return nil, qErr
			}
			count := 0
			var lastRowID int64
			for qRows.Next() {
				count++
				var rowID int64
				var idx sql.NullInt64
				var blob []byte
				if err := qRows.Scan(&rowID, &idx, &blob); err != nil {
					badRows++
					continue
				}
				lastRowID = rowID
				if !idx.Valid {
					badRows++
					continue
				}
				if isStep && len(blob) == 0 {
					continue
				}
				entry, err := parseAgyMetadata(blob, isStep)
				if err != nil {
					badRows++
					continue
				}
				entries = append(entries, agyIndexedMetadata{idx: int(idx.Int64), entry: entry})
			}
			_ = qRows.Close()
			if count < pageRows {
				break
			}
			after = lastRowID
			hasAfter = true
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].idx < entries[j].idx })
		return entries, nil
	}

	var generations []agyIndexedMetadata
	if tables["gen_metadata"] {
		generations, err = readMeta(
			"SELECT rowid AS row_id, idx, data FROM gen_metadata WHERE rowid > ? ORDER BY rowid LIMIT ?",
			64,
			false,
		)
		if err != nil {
			return nil, nil, badRows, err
		}
	}

	var trajectoryTs *int64
	if tables["trajectory_metadata_blob"] {
		tRows, err := db.Query("SELECT data FROM trajectory_metadata_blob")
		if err == nil {
			for tRows.Next() {
				var blob []byte
				if err := tRows.Scan(&blob); err == nil && len(blob) > 0 && trajectoryTs == nil {
					if root, err := parseProtobufFields(blob); err == nil {
						trajectoryTs = pbTimestampMs(pbNested(root, 2))
					}
				}
			}
			_ = tRows.Close()
		}
	}

	var steps []agyIndexedMetadata
	if tables["steps"] {
		steps, err = readMeta(
			"SELECT rowid AS row_id, idx, metadata AS data FROM steps WHERE rowid > ? ORDER BY rowid LIMIT ?",
			64,
			true,
		)
		if err != nil {
			return nil, nil, badRows, err
		}
	}

	models := map[int]agyLearnedModel{}
	for _, g := range generations {
		if g.entry.modelID > 0 && isConcreteAntigravityModel(g.entry.model) {
			rememberAgyModel(models, g.entry.modelID, agyLearnedModel{name: g.entry.model})
		}
	}
	for _, s := range steps {
		if s.entry.modelID > 0 && isConcreteAntigravityModel(s.entry.model) {
			rememberAgyModel(models, s.entry.modelID, agyLearnedModel{name: s.entry.model})
		}
	}

	cands := buildAgyUsageCandidates(sessionID, generations, steps, trajectoryTs, fallbackTimestampMs)
	return cands, models, badRows, nil
}

type agyUsageCandidate struct {
	sessionID           string
	timestampMs         int64
	model               string
	modelID             int
	modelQuality        int
	timestampQuality    int
	dedupeKey           string
	keys                []string
	uncachedInputTokens int
	cachedInputTokens   int
	cacheCreationTokens int
	outputTokens        int
	reasoningTokens     int
}

func buildAgyUsageCandidates(
	sessionID string,
	generations []agyIndexedMetadata,
	steps []agyIndexedMetadata,
	trajectoryTs *int64,
	fallbackTs int64,
) []agyUsageCandidate {
	genByIdx := make(map[int]agyMetadata, len(generations))
	for _, g := range generations {
		genByIdx[g.idx] = g.entry
	}

	var out []agyUsageCandidate
	process := func(source string, entries []agyIndexedMetadata) {
		for index, item := range entries {
			entry := item.entry
			for usageIdx, usage := range entry.usages {
				modelID := pbNumberAt(usage, 1)
				if modelID == 0 {
					modelID = entry.modelID
				}
				var gen *agyMetadata
				if source == "step" {
					if g, ok := genByIdx[item.idx]; ok {
						gen = &g
					}
				}
				matches := func(m agyMetadata) bool {
					return modelID == 0 || m.modelID == modelID
				}
				modelName := ""
				if matches(entry) && entry.model != "" {
					modelName = entry.model
				} else if gen != nil && matches(*gen) {
					modelName = gen.model
				}

				uncachedIn := pbNumberAt(usage, 2)
				cachedIn := pbNumberAt(usage, 5)
				cacheCreate := pbNumberAt(usage, 4)
				reasoningRaw := pbNumberAt(usage, 9)
				visibleOut := pbNumberAt(usage, 10)
				outTok := max(pbNumberAt(usage, 3), reasoningRaw+visibleOut)
				reasoningTok := min(outTok, reasoningRaw)

				if uncachedIn+cachedIn+cacheCreate+outTok == 0 {
					continue
				}

				var keys []string
				for _, fieldKey := range []int{11, 12, 7} {
					if id := pbTextAt(usage, fieldKey); id != "" {
						keys = append(keys, fmt.Sprintf("antigravity:%d:%s", fieldKey, id))
					}
				}

				dedupeKey := fmt.Sprintf("antigravity:%s:%s:%d:%d", sessionID, source, index, usageIdx)
				if len(keys) > 0 {
					dedupeKey = keys[0]
				}

				tsMs := fallbackTs
				tsQuality := 0
				if entry.timestampMs != nil {
					tsMs = *entry.timestampMs
					tsQuality = 2
				} else if trajectoryTs != nil {
					tsMs = *trajectoryTs
					tsQuality = 1
				}

				mq := 0
				if isConcreteAntigravityModel(modelName) {
					mq = 4
				} else if modelName != "" {
					mq = 1
				}

				out = append(out, agyUsageCandidate{
					sessionID:           sessionID,
					timestampMs:         tsMs,
					model:               modelName,
					modelID:             modelID,
					modelQuality:        mq,
					timestampQuality:    tsQuality,
					dedupeKey:           dedupeKey,
					keys:                keys,
					uncachedInputTokens: uncachedIn,
					cachedInputTokens:   cachedIn,
					cacheCreationTokens: cacheCreate,
					outputTokens:        outTok,
					reasoningTokens:     reasoningTok,
				})
			}
		}
	}

	process("step", steps)
	process("generation", generations)
	return out
}

type agyUFNode struct {
	agyUsageCandidate
	parent int
	size   int
	owner  int
}

type agyUnionFind struct {
	groups     []agyUFNode
	identities map[string]int
}

func newAgyUnionFind() *agyUnionFind {
	return &agyUnionFind{identities: make(map[string]int)}
}

func (u *agyUnionFind) find(idx int) int {
	root := idx
	for u.groups[root].parent != root {
		root = u.groups[root].parent
	}
	for idx != root {
		next := u.groups[idx].parent
		u.groups[idx].parent = root
		idx = next
	}
	return root
}

func (u *agyUnionFind) merge(left, right int) int {
	a := u.find(left)
	b := u.find(right)
	if a == b {
		return a
	}
	if u.groups[a].size < u.groups[b].size {
		a, b = b, a
	}
	target := &u.groups[a]
	source := &u.groups[b]

	first := target
	if source.owner < target.owner {
		first = source
	}
	bestModel := target
	if source.modelQuality == target.modelQuality {
		bestModel = first
	} else if source.modelQuality > target.modelQuality {
		bestModel = source
	}
	bestTime := target
	if source.timestampQuality > target.timestampQuality ||
		(source.timestampQuality == target.timestampQuality && source.timestampMs < target.timestampMs) {
		bestTime = source
	}

	target.sessionID = first.sessionID
	target.dedupeKey = first.dedupeKey
	target.model = bestModel.model
	target.modelQuality = bestModel.modelQuality
	target.timestampMs = bestTime.timestampMs
	target.timestampQuality = bestTime.timestampQuality
	target.uncachedInputTokens = max(target.uncachedInputTokens, source.uncachedInputTokens)
	target.cachedInputTokens = max(target.cachedInputTokens, source.cachedInputTokens)
	target.cacheCreationTokens = max(target.cacheCreationTokens, source.cacheCreationTokens)
	target.outputTokens = max(target.outputTokens, source.outputTokens)
	target.reasoningTokens = max(target.reasoningTokens, source.reasoningTokens)
	target.owner = first.owner
	target.size += source.size
	source.parent = a
	return a
}

func (u *agyUnionFind) append(c agyUsageCandidate) {
	idx := len(u.groups)
	u.groups = append(u.groups, agyUFNode{
		agyUsageCandidate: c,
		parent:            idx,
		size:              1,
		owner:             idx,
	})
	for _, k := range c.keys {
		if existing, ok := u.identities[k]; ok {
			u.merge(idx, existing)
		}
		u.identities[k] = idx
	}
}
