package commands

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandTrack_NeverPerformsUpdateWork is a structural guard, not a
// behavioural test.
//
// `track` runs inside the shell hook on every command. It must never download,
// extract, verify, or swap a binary — all of that belongs to the daemon, with
// `gc` (once per new shell) applying whatever the daemon staged. The most track
// may do is start a stopped daemon service.
//
// Behavioural tests can only prove that today's code path does not update.
// Parsing the call graph proves nobody can reintroduce it without deleting this
// test, which is the point: the constraint is easy to violate by accident,
// because the update helpers live in the same package.
func TestCommandTrack_NeverPerformsUpdateWork(t *testing.T) {
	fset := token.NewFileSet()
	pkg, err := parser.ParseDir(fset, ".", nil, 0)
	require.NoError(t, err)

	files := map[string]*ast.File{}
	for _, p := range pkg {
		for name, f := range p.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			files[name] = f
		}
	}
	require.NotEmpty(t, files)

	// Index every top-level func in the package so we can walk transitively.
	funcs := map[string]*ast.FuncDecl{}
	for _, f := range files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				funcs[fn.Name.Name] = fn
			}
		}
	}
	require.Contains(t, funcs, "commandTrack")

	// Anything that downloads, unpacks, verifies or swaps a binary. If track can
	// reach one of these, the shell hook can be made to do update work.
	forbidden := map[string]string{
		"ApplyUpdate":                   "downloads and installs a release",
		"DownloadAndVerify":             "downloads a release archive",
		"ExtractBinaries":               "unpacks a release archive",
		"ReplaceBinary":                 "swaps a binary in place",
		"ReplaceBinaryWithBackupSuffix": "swaps a binary in place",
		"RestoreBinaryBackup":           "swaps a binary in place",
		"FetchLatestCLIRelease":         "makes a network call to look for updates",
		"FetchLatestVersion":            "makes a network call to look for updates",
		"FetchChecksum":                 "makes a network call to look for updates",
		"FetchChecksumFrom":             "makes a network call to look for updates",
		"EnsureDaemonBinary":            "downloads the daemon binary",
		"commandDaemonApplyUpdate":      "applies a staged update",
		"launchDetachedApplyUpdate":     "spawns the update-applying path",
		"commandUpdate":                 "is the interactive updater",
	}

	// Walk the call graph from commandTrack.
	seen := map[string]bool{}
	var path []string
	var violations []string

	var walk func(name string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true

		fn, ok := funcs[name]
		if !ok {
			return // defined in another package; the selector check below covers those
		}

		path = append(path, name)
		defer func() { path = path[:len(path)-1] }()

		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			var callee string
			switch f := call.Fun.(type) {
			case *ast.Ident: // localFunc(...)
				callee = f.Name
			case *ast.SelectorExpr: // model.Foo(...)
				callee = f.Sel.Name
			default:
				return true
			}

			if why, bad := forbidden[callee]; bad {
				violations = append(violations,
					strings.Join(append(path, callee), " -> ")+"  ("+callee+" "+why+")")
				return true
			}
			walk(callee)
			return true
		})
	}
	walk("commandTrack")

	assert.Empty(t, violations,
		"commandTrack must never reach update machinery; the most it may do is start the daemon.\n"+
			"Offending call paths:\n  "+strings.Join(violations, "\n  "))
}

// The one daemon action track is allowed to take must remain reachable.
func TestCommandTrack_CanStillStartTheDaemon(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "track.go", nil, 0)
	require.NoError(t, err)

	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "maybeStartDaemonFromTrack" {
			found = true
		}
		return true
	})
	assert.True(t, found, "track should still be able to bring a stopped daemon back up")
}
