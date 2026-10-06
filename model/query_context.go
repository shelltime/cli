package model

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// QueryContext is the environment snapshot `shelltime q` sends with a
// command-suggestion request when ai.shareContext is enabled. JSON names are
// shared with the server's CommandSuggestionContext.
type QueryContext struct {
	System  *QuerySystemContext  `json:"system,omitempty"`
	Git     *QueryGitContext     `json:"git,omitempty"`
	Project *QueryProjectContext `json:"project,omitempty"`
	Tools   []string             `json:"tools,omitempty"`
	Dir     *QueryDirContext     `json:"dir,omitempty"`
}

// QuerySystemContext describes the machine and terminal session.
type QuerySystemContext struct {
	OSVersion   string    `json:"osVersion,omitempty"`
	Kernel      string    `json:"kernel,omitempty"`
	Arch        string    `json:"arch,omitempty"`
	CPUCount    int       `json:"cpuCount,omitempty"`
	UptimeSec   int64     `json:"uptimeSec,omitempty"`
	LoadAvg     []float64 `json:"loadAvg,omitempty"`
	IsRoot      bool      `json:"isRoot,omitempty"`
	SSH         bool      `json:"ssh,omitempty"`
	Container   string    `json:"container,omitempty"`
	Multiplexer string    `json:"multiplexer,omitempty"`
	TermProgram string    `json:"termProgram,omitempty"`
	Display     string    `json:"display,omitempty"`
	Timezone    string    `json:"timezone,omitempty"`
	LocalTime   string    `json:"localTime,omitempty"`
}

// QueryGitContext describes the git repository the query runs in. When HEAD
// is detached, Branch holds the short commit hash.
type QueryGitContext struct {
	PathInRepo       string   `json:"pathInRepo,omitempty"`
	Branch           string   `json:"branch,omitempty"`
	Detached         bool     `json:"detached,omitempty"`
	Upstream         string   `json:"upstream,omitempty"`
	Ahead            int      `json:"ahead,omitempty"`
	Behind           int      `json:"behind,omitempty"`
	Staged           int      `json:"staged,omitempty"`
	Unstaged         int      `json:"unstaged,omitempty"`
	Untracked        int      `json:"untracked,omitempty"`
	Conflicted       int      `json:"conflicted,omitempty"`
	Operation        string   `json:"operation,omitempty"`
	StatusIncomplete bool     `json:"statusIncomplete,omitempty"`
	Remotes          []string `json:"remotes,omitempty"`
	RecentCommits    []string `json:"recentCommits,omitempty"`
}

// QueryProjectContext lists project tooling found in manifest and lock files.
// Script, target and recipe lists hold names only.
type QueryProjectContext struct {
	Types           []string `json:"types,omitempty"`
	PackageManagers []string `json:"packageManagers,omitempty"`
	PackageScripts  []string `json:"packageScripts,omitempty"`
	MakeTargets     []string `json:"makeTargets,omitempty"`
	JustRecipes     []string `json:"justRecipes,omitempty"`
}

// QueryDirContext lists entry names in the working directory.
type QueryDirContext struct {
	Entries   []string `json:"entries,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
}

const (
	// QueryContextMaxRunes caps most free-text context values.
	QueryContextMaxRunes = 200

	queryDirMaxEntries     = 40
	queryDirReadLimit      = 256
	queryScriptsMax        = 20
	queryProjectMaxDepth   = 8
	queryPackageJSONMaxLen = 512 * 1024
	queryTaskFileMaxLen    = 256 * 1024
)

// SanitizeContextString removes ANSI escape sequences and control
// characters, collapses whitespace, and truncates s to maxRunes runes.
func SanitizeContextString(s string, maxRunes int) string {
	if s == "" {
		return ""
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}

	const esc = 0x1b
	var b strings.Builder
	pendingSpace := false
	runes := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == esc {
			i = skipANSISequence(rs, i)
			pendingSpace = true
			continue
		}
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			pendingSpace = true
			continue
		}
		if pendingSpace && b.Len() > 0 {
			b.WriteByte(' ')
			runes++
		}
		pendingSpace = false
		if runes >= maxRunes {
			return strings.TrimSpace(b.String()) + "…"
		}
		b.WriteRune(r)
		runes++
	}
	return b.String()
}

// skipANSISequence returns the index of the last rune of the escape sequence
// starting at rs[i] (an ESC): CSI sequences run to a final byte in @-~, OSC
// sequences to BEL or ESC-backslash, anything else is ESC plus one rune.
func skipANSISequence(rs []rune, i int) int {
	if i+1 >= len(rs) {
		return i
	}
	switch rs[i+1] {
	case '[':
		for j := i + 2; j < len(rs); j++ {
			if rs[j] >= '@' && rs[j] <= '~' {
				return j
			}
		}
		return len(rs) - 1
	case ']':
		for j := i + 2; j < len(rs); j++ {
			if rs[j] == 0x07 {
				return j
			}
			if rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\' {
				return j + 1
			}
		}
		return len(rs) - 1
	default:
		return i + 1
	}
}

// ListDir returns up to 40 entry names of dir, sorted, with directories
// suffixed by "/". Only the first 256 entries the OS returns are considered,
// so huge directories stay cheap.
func ListDir(dir string) *QueryDirContext {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()

	entries, err := f.ReadDir(queryDirReadLimit)
	if err != nil && err != io.EOF {
		return nil
	}
	truncated := len(entries) == queryDirReadLimit

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if name == ".git" || name == ".DS_Store" {
			continue
		}
		name = SanitizeContextString(name, 100)
		if name == "" {
			continue
		}
		if e.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	if len(names) > queryDirMaxEntries {
		names = names[:queryDirMaxEntries]
		truncated = true
	}
	return &QueryDirContext{Entries: names, Truncated: truncated}
}

// nodeLockfiles map lock files to their package manager, in priority order.
var nodeLockfiles = []struct{ file, manager string }{
	{"pnpm-lock.yaml", "pnpm"},
	{"bun.lock", "bun"},
	{"bun.lockb", "bun"},
	{"yarn.lock", "yarn"},
	{"package-lock.json", "npm"},
	{"npm-shrinkwrap.json", "npm"},
}

var pythonLockfiles = []struct{ file, manager string }{
	{"uv.lock", "uv"},
	{"poetry.lock", "poetry"},
	{"pdm.lock", "pdm"},
	{"Pipfile.lock", "pipenv"},
	{"Pipfile", "pipenv"},
	{"requirements.txt", "pip"},
}

// projectMarkers map manifest files to a project type and, when the type
// alone does not imply it, a package manager or runner.
var projectMarkers = []struct{ file, kind, manager string }{
	{"go.mod", "go", ""},
	{"Cargo.toml", "rust", ""},
	{"package.json", "node", ""},
	{"deno.json", "deno", ""},
	{"deno.jsonc", "deno", ""},
	{"pyproject.toml", "python", ""},
	{"requirements.txt", "python", ""},
	{"Pipfile", "python", ""},
	{"Gemfile", "ruby", "bundler"},
	{"pom.xml", "java", "maven"},
	{"build.gradle", "java", "gradle"},
	{"build.gradle.kts", "kotlin", "gradle"},
	{"composer.json", "php", "composer"},
	{"mix.exs", "elixir", "mix"},
	{"CMakeLists.txt", "cmake", ""},
	{"Dockerfile", "docker", ""},
	{"compose.yaml", "docker-compose", ""},
	{"compose.yml", "docker-compose", ""},
	{"docker-compose.yaml", "docker-compose", ""},
	{"docker-compose.yml", "docker-compose", ""},
	{"flake.nix", "nix", ""},
	{"shell.nix", "nix", ""},
	{"Taskfile.yml", "taskfile", "task"},
	{"Taskfile.yaml", "taskfile", "task"},
}

var makefileNames = []string{"GNUmakefile", "makefile", "Makefile"}
var justfileNames = []string{"justfile", "Justfile", ".justfile"}

// commonScripts are listed first when a project has more scripts than fit.
var commonScripts = []string{
	"dev", "start", "build", "test", "lint", "format", "fmt", "typecheck",
	"check", "serve", "preview", "watch", "clean", "install", "deploy", "release",
}

// DetectProject inspects cwd and its parents (up to the repository root,
// marked by a .git entry, and never home itself) for manifest, lock and task
// files. The nearest directory wins for package managers and script lists,
// so a package inside a monorepo still picks up the workspace lock file.
func DetectProject(cwd, home string) *QueryProjectContext {
	if cwd == "" {
		return nil
	}
	p := &QueryProjectContext{}
	var nodeManager, pythonManager string
	seenPackageJSON, seenMakefile, seenJustfile := false, false, false

	dir := filepath.Clean(cwd)
	for depth := 0; depth < queryProjectMaxDepth; depth++ {
		if home != "" && dir == filepath.Clean(home) {
			break
		}
		names := dirEntryNames(dir)

		for _, m := range projectMarkers {
			if names[m.file] {
				p.Types = appendUnique(p.Types, m.kind)
				if m.manager != "" {
					p.PackageManagers = appendUnique(p.PackageManagers, m.manager)
				}
			}
		}

		if names["package.json"] && !seenPackageJSON {
			seenPackageJSON = true
			scripts, manager := readPackageJSON(filepath.Join(dir, "package.json"))
			p.PackageScripts = scripts
			if nodeManager == "" {
				nodeManager = manager
			}
		}
		if nodeManager == "" {
			for _, l := range nodeLockfiles {
				if names[l.file] {
					nodeManager = l.manager
					break
				}
			}
		}
		if pythonManager == "" {
			for _, l := range pythonLockfiles {
				if names[l.file] {
					pythonManager = l.manager
					break
				}
			}
		}
		if name := firstPresent(names, makefileNames); name != "" && !seenMakefile {
			seenMakefile = true
			p.MakeTargets = ParseMakeTargets(readCapped(filepath.Join(dir, name), queryTaskFileMaxLen))
			p.Types = appendUnique(p.Types, "make")
		}
		if name := firstPresent(names, justfileNames); name != "" && !seenJustfile {
			seenJustfile = true
			p.JustRecipes = ParseJustRecipes(readCapped(filepath.Join(dir, name), queryTaskFileMaxLen))
			p.Types = appendUnique(p.Types, "just")
		}

		parent := filepath.Dir(dir)
		if names[".git"] || parent == dir {
			break
		}
		dir = parent
	}

	if nodeManager != "" {
		p.PackageManagers = appendUnique(p.PackageManagers, nodeManager)
	}
	if pythonManager != "" {
		p.PackageManagers = appendUnique(p.PackageManagers, pythonManager)
	}
	if len(p.Types)+len(p.PackageManagers)+len(p.PackageScripts)+len(p.MakeTargets)+len(p.JustRecipes) == 0 {
		return nil
	}
	return p
}

func dirEntryNames(dir string) map[string]bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names
}

func firstPresent(names map[string]bool, candidates []string) string {
	for _, c := range candidates {
		if names[c] {
			return c
		}
	}
	return ""
}

func readCapped(path string, maxLen int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxLen))
	if err != nil {
		return ""
	}
	return string(b)
}

// readPackageJSON returns the script names and the package manager named by
// the "packageManager" field (e.g. "pnpm@9.1.0" yields "pnpm").
func readPackageJSON(path string) ([]string, string) {
	content := readCapped(path, queryPackageJSONMaxLen)
	if content == "" {
		return nil, ""
	}
	var pkg struct {
		Scripts        map[string]json.RawMessage `json:"scripts"`
		PackageManager string                     `json:"packageManager"`
	}
	if err := json.Unmarshal([]byte(content), &pkg); err != nil {
		return nil, ""
	}

	manager := ""
	if name, _, _ := strings.Cut(pkg.PackageManager, "@"); name != "" {
		manager = SanitizeContextString(name, 32)
	}

	names := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		names = append(names, name)
	}
	return prioritizeScripts(names), manager
}

// prioritizeScripts sanitizes and de-duplicates names, puts common entry
// points first and the rest in alphabetical order, and keeps at most 20.
func prioritizeScripts(names []string) []string {
	clean := make([]string, 0, len(names))
	for _, n := range names {
		if s := SanitizeContextString(n, 64); s != "" && !slices.Contains(clean, s) {
			clean = append(clean, s)
		}
	}
	sort.Strings(clean)

	out := make([]string, 0, min(len(clean), queryScriptsMax))
	for _, c := range commonScripts {
		if slices.Contains(clean, c) {
			out = append(out, c)
		}
	}
	for _, n := range clean {
		if len(out) >= queryScriptsMax {
			break
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	if len(out) > queryScriptsMax {
		out = out[:queryScriptsMax]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParseMakeTargets extracts explicit target names from a Makefile. Variable
// assignments, special targets (.PHONY), pattern rules, targets built from
// variables and file targets containing "/" are skipped.
func ParseMakeTargets(content string) []string {
	var targets []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '\t' || line[0] == ' ' || line[0] == '#' {
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}
		rest := line[idx+1:]
		// FOO := x, FOO ::= x and FOO :::= x are assignments, not rules
		if strings.HasPrefix(strings.TrimLeft(rest, ":"), "=") {
			continue
		}
		head := line[:idx]
		if strings.ContainsAny(head, "=%$") {
			continue
		}
		for _, t := range strings.Fields(head) {
			if strings.HasPrefix(t, ".") || strings.Contains(t, "/") {
				continue
			}
			targets = append(targets, t)
		}
	}
	return prioritizeScripts(targets)
}

// ParseJustRecipes extracts public recipe names from a justfile. Settings,
// aliases, imports, attributes, assignments and private recipes (leading
// underscore) are skipped.
func ParseJustRecipes(content string) []string {
	var recipes []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '\t' || line[0] == ' ' || line[0] == '#' || line[0] == '[' {
			continue
		}
		if strings.Contains(line, ":=") {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "set", "alias", "import", "mod", "export":
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}
		head := strings.Fields(line[:idx])
		if len(head) == 0 {
			continue
		}
		name := strings.TrimPrefix(head[0], "@")
		if name == "" || strings.HasPrefix(name, "_") || !isRecipeName(name) {
			continue
		}
		recipes = append(recipes, name)
	}
	return prioritizeScripts(recipes)
}

func isRecipeName(name string) bool {
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}
	return true
}
