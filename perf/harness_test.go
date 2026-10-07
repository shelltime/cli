//go:build unix

package perf

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// daemonSocket mirrors model.DefaultSocketPath. track's fast path stats this
// fixed path, so the fake daemon has to listen exactly here.
const daemonSocket = "/tmp/shelltime.sock"

// The command every track scenario reports, in the shape the zsh hook sends it
// (model/hooks/zsh.zsh).
const (
	benchUser    = "bench"
	benchShell   = "zsh"
	benchSession = int64(1759800000)
	benchCommand = "git status --short"
	benchPPID    = 4242
)

const (
	// seedPairs is how many already-synced pre/post pairs sit in the txt store.
	// Nothing prunes the store between two `shelltime gc` runs (one per new
	// shell), so a busy shell carries a few hundred, and the direct post path
	// reads all of them on every command.
	seedPairs = 500
	// flushCount is the config's flush threshold. post-sync seeds
	// flushCount-1 pending pairs so every measured post triggers a sync.
	flushCount = 10
	// maxFixtureBytes keeps each store file well under the 512 KiB scanner
	// buffer in model/db.go, past which reading post.txt corrupts lines.
	maxFixtureBytes = 400 << 10
	// warmupRuns untimed runs per scenario warm the page cache and the
	// sandbox, as on a user's machine where the binary runs every command.
	warmupRuns = 5
)

// baseConfig is a typical logged-in config. The endpoint is a closed local
// port: if a scenario that must not sync ever does, the request fails, logs an
// error and requireNoErrors fails the benchmark.
const baseConfig = `token: bench-token
apiEndpoint: http://127.0.0.1:9
flushCount: 10
`

var (
	// workDir holds per-process scratch files: the lazily built binary and
	// the lsb_release stub.
	workDir string
	devNull *os.File

	binOnce sync.Once
	binPath string
	binErr  error

	pathOnce  sync.Once
	childPATH string
	pathErr   error
)

func TestMain(m *testing.M) {
	os.Exit(func() int {
		dir, err := os.MkdirTemp("", "shelltime-perf-")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer os.RemoveAll(dir)
		workDir = dir

		devNull, err = os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer devNull.Close()

		return m.Run()
	}())
}

// shelltimeBin returns the binary under test: $SHELLTIME_BENCH_BIN, or the
// CLI built from this checkout with release-like flags on first use, so a
// plain `go test` that runs no benchmark never pays for the build.
func shelltimeBin(b *testing.B) string {
	b.Helper()
	binOnce.Do(func() {
		if p := os.Getenv("SHELLTIME_BENCH_BIN"); p != "" {
			if binPath, binErr = filepath.Abs(p); binErr == nil {
				_, binErr = os.Stat(binPath)
			}
			return
		}
		root, err := repoRoot()
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(workDir, "shelltime")
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false",
			"-ldflags", "-s -w -X main.version=perf", "-o", binPath, "./cmd/cli")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			binErr = fmt.Errorf("build shelltime: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		b.Fatal(binErr)
	}
	return binPath
}

// repoRoot is the CLI module this module sits in.
func repoRoot() (string, error) {
	root, err := filepath.Abs("..")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		if strings.TrimSpace(line) == "module github.com/malamtime/cli" {
			return root, nil
		}
	}
	return "", fmt.Errorf("%s is not the shelltime CLI module; set SHELLTIME_BENCH_BIN", root)
}

// childPath is the PATH the binary runs with. On Linux the direct post path
// execs `lsb_release -a` (model.GetOSAndVersion), which is a Python script on
// Ubuntu: tens of noisy milliseconds that would bury any change in shelltime
// itself. A shell stub with the same output keeps the exec but not the noise.
// SHELLTIME_BENCH_REAL_OSINFO=1 measures the real one.
func childPath(b *testing.B) string {
	b.Helper()
	pathOnce.Do(func() {
		childPATH = os.Getenv("PATH")
		if runtime.GOOS != "linux" || os.Getenv("SHELLTIME_BENCH_REAL_OSINFO") != "" {
			return
		}
		stubDir := filepath.Join(workDir, "stub")
		if pathErr = os.MkdirAll(stubDir, 0o755); pathErr != nil {
			return
		}
		stub := "#!/bin/sh\nprintf 'Distributor ID:\\tUbuntu\\nDescription:\\tUbuntu 24.04 LTS\\nRelease:\\t24.04\\nCodename:\\tnoble\\n'\n"
		pathErr = os.WriteFile(filepath.Join(stubDir, "lsb_release"), []byte(stub), 0o755)
		childPATH = stubDir + string(os.PathListSeparator) + childPATH
	})
	if pathErr != nil {
		b.Fatal(pathErr)
	}
	return childPATH
}

// sandbox is an isolated $HOME with a .shelltime/config.yaml.
type sandbox struct {
	b    *testing.B
	bin  string
	home string
	env  []string
}

func newSandbox(b *testing.B, bin, config string) *sandbox {
	b.Helper()
	home := b.TempDir()
	s := &sandbox{
		b:    b,
		bin:  bin,
		home: home,
		env:  []string{"HOME=" + home, "USER=" + benchUser, "PATH=" + childPath(b), "LANG=C"},
	}
	if config != "" {
		if err := os.MkdirAll(s.path("commands"), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(s.path("config.yaml"), []byte(config), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return s
}

// path joins elem under the sandbox's ~/.shelltime.
func (s *sandbox) path(elem ...string) string {
	return filepath.Join(append([]string{s.home, ".shelltime"}, elem...)...)
}

type runStat struct {
	wall, user, sys time.Duration
	rss             int64 // peak resident set, bytes
}

// run execs the binary once with stdio on /dev/null, like the hooks' &>/dev/null.
func (s *sandbox) run(args ...string) runStat {
	cmd := exec.Command(s.bin, args...)
	cmd.Env = s.env
	cmd.Dir = s.home
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	start := time.Now()
	err := cmd.Run()
	wall := time.Since(start)
	if err != nil {
		s.b.Fatalf("%s %s: %v", filepath.Base(s.bin), strings.Join(args, " "), err)
	}
	st := runStat{wall: wall, user: cmd.ProcessState.UserTime(), sys: cmd.ProcessState.SystemTime()}
	if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
		st.rss = int64(ru.Maxrss)
		if runtime.GOOS != "darwin" { // Linux reports KiB, macOS bytes.
			st.rss *= 1024
		}
	}
	return st
}

// requireNoErrors fails the benchmark if the CLI logged an error. track exits 0
// whatever happens, so a run that broke early would otherwise just look fast.
func (s *sandbox) requireNoErrors() {
	s.b.Helper()
	f, err := os.Open(s.path("log.log"))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.b.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "level=ERROR") {
			s.b.Fatalf("shelltime logged an error: %s", sc.Text())
		}
	}
}

// measure does warmupRuns untimed runs, then times one exec per iteration.
// reset, if set, runs before every exec with the timer stopped, so cost does
// not drift as the store grows. It returns the total number of execs.
func measure(b *testing.B, reset func(), run func() runStat) int {
	b.Helper()
	for range warmupRuns {
		if reset != nil {
			reset()
		}
		run()
	}
	var stats []runStat
	for b.Loop() {
		if reset != nil {
			b.StopTimer()
			reset()
			b.StartTimer()
		}
		stats = append(stats, run())
	}
	report(b, stats)
	return warmupRuns + len(stats)
}

// report adds per-exec CPU time, wall-time percentiles and peak RSS next to
// ns/op. These are the child's numbers, which is why -benchmem is not used:
// it would measure the harness.
func report(b *testing.B, stats []runStat) {
	if len(stats) == 0 {
		return
	}
	n := float64(len(stats))
	walls := make([]time.Duration, len(stats))
	var user, sys time.Duration
	var rss int64
	for i, st := range stats {
		walls[i] = st.wall
		user += st.user
		sys += st.sys
		rss += st.rss
	}
	slices.Sort(walls)
	pct := func(p float64) float64 {
		return float64(walls[int(p*float64(len(walls)-1))].Nanoseconds())
	}
	b.ReportMetric(float64(user.Nanoseconds())/n, "user-ns/op")
	b.ReportMetric(float64(sys.Nanoseconds())/n, "sys-ns/op")
	b.ReportMetric(pct(0.50), "p50-ns/op")
	b.ReportMetric(pct(0.95), "p95-ns/op")
	b.ReportMetric(float64(rss)/n, "peak-rss-B")
}

// trackArgs is a hook call: model/hooks/zsh.zsh passes exactly these flags.
func trackArgs(phase string) []string {
	args := []string{"track", "-s=" + benchShell, "-id=" + strconv.FormatInt(benchSession, 10),
		"-cmd=" + benchCommand, "-p=" + phase, "--ppid=" + strconv.Itoa(benchPPID)}
	if phase == "post" {
		args = append(args, "-r=0")
	}
	return args
}

// requireNoDaemon skips while something owns the daemon socket: a real daemon
// would receive the fake commands, and the direct scenarios would take the
// daemon fast path. A stale socket file left by a dead daemon is removed.
func requireNoDaemon(b *testing.B) {
	b.Helper()
	if _, err := os.Lstat(daemonSocket); errors.Is(err, os.ErrNotExist) {
		return
	}
	if conn, err := net.DialTimeout("unix", daemonSocket, 50*time.Millisecond); err == nil {
		conn.Close()
		b.Skipf("a shelltime daemon is listening on %s; stop it to run the track benchmarks", daemonSocket)
	}
	if err := os.Remove(daemonSocket); err != nil {
		b.Skipf("cannot remove stale %s: %v", daemonSocket, err)
	}
}

// fakeDaemon accepts on the daemon socket, reads each message to EOF (the CLI
// writes one JSON message and closes) and counts the track events it gets.
type fakeDaemon struct {
	ln     *net.UnixListener
	served chan struct{}
	conns  sync.WaitGroup

	mu     sync.Mutex
	events map[string]int // by message type; "invalid" for anything unexpected
}

func startFakeDaemon(b *testing.B) *fakeDaemon {
	b.Helper()
	requireNoDaemon(b)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: daemonSocket, Net: "unix"})
	if err != nil {
		b.Fatalf("listen on %s: %v", daemonSocket, err)
	}
	d := &fakeDaemon{ln: ln, served: make(chan struct{}), events: map[string]int{}}
	go d.serve()
	b.Cleanup(func() {
		ln.Close() // also unlinks the socket file
		<-d.served
		d.conns.Wait()
	})
	return d
}

func (d *fakeDaemon) serve() {
	defer close(d.served)
	for {
		conn, err := d.ln.Accept()
		if err != nil {
			return
		}
		d.conns.Add(1)
		go func() {
			defer d.conns.Done()
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			data, _ := io.ReadAll(conn)
			var msg struct {
				Type    string `json:"type"`
				Payload struct {
					Command struct {
						Cmd string `json:"cmd"`
					} `json:"command"`
				} `json:"payload"`
			}
			kind := "invalid"
			if json.Unmarshal(data, &msg) == nil && msg.Payload.Command.Cmd == benchCommand {
				kind = msg.Type
			}
			d.mu.Lock()
			d.events[kind]++
			d.mu.Unlock()
		}()
	}
}

// expect fails unless exactly runs events of msgType, and nothing else, arrived.
func (d *fakeDaemon) expect(b *testing.B, msgType string, runs int) {
	b.Helper()
	snapshot := func() (got, total int) {
		d.mu.Lock()
		defer d.mu.Unlock()
		for k, n := range d.events {
			total += n
			if k == msgType {
				got = n
			}
		}
		return got, total
	}
	deadline := time.Now().Add(2 * time.Second)
	got, total := snapshot()
	for got < runs && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		got, total = snapshot()
	}
	if got != runs || total != runs {
		b.Fatalf("fake daemon got %d %q events and %d others for %d runs; track did not take the daemon path",
			got, msgType, total-got, runs)
	}
}

// fakeAPI stands in for the server's POST /api/v1/track. It answers 204, one
// of the two statuses model.SendHTTPRequestJSON accepts.
type fakeAPI struct {
	srv       *httptest.Server
	syncs     atomic.Int64 // batches of exactly flushCount records
	unexpects atomic.Int64
}

func startFakeAPI(b *testing.B) *fakeAPI {
	b.Helper()
	a := &fakeAPI{}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Data []json.RawMessage `json:"data"`
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/track" ||
			json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Data) != flushCount {
			a.unexpects.Add(1)
		} else {
			a.syncs.Add(1)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	b.Cleanup(a.srv.Close)
	return a
}

// storedCommand mirrors the JSON of model.Command, as the txt store holds it.
type storedCommand struct {
	Shell     string    `json:"shell"`
	SessionID int64     `json:"sid"`
	Command   string    `json:"cmd"`
	Main      string    `json:"main"`
	Hostname  string    `json:"hn"`
	Username  string    `json:"un"`
	Time      time.Time `json:"t"`
	EndTime   time.Time `json:"et"`
	Result    int       `json:"result"`
	Phase     int       `json:"phase"`
	PPID      int       `json:"ppid,omitempty"`
}

// storeLine mirrors model.Command.ToLine: JSON, a tab, the recording time in
// Unix nanoseconds, a newline.
func storeLine(c storedCommand, recorded time.Time) []byte {
	buf, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	buf = append(buf, '\t')
	buf = strconv.AppendInt(buf, recorded.UnixNano(), 10)
	return append(buf, '\n')
}

// seedCommands is the mix the store is seeded with. Commands repeat, as in a
// real history, so the store's per-command groups have many entries.
var seedCommands = []string{
	"ls -la", "git status", "git diff --stat", "go test ./...", "cd ~/projects/shelltime",
	"kubectl get pods -n default", "docker ps -a", "vim main.go", "make build",
	"curl -H 'Authorization: Bearer abc123' https://example.com/api", "npm run dev",
	"rg TODO --type go", "export GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx", "htop",
}

// storeFixture is a seeded txt store and the state to restore it to.
type storeFixture struct {
	s        *sandbox
	postSize int64
	cursor   []byte
}

// seedStore writes synced pre/post pairs the cursor already covers, then
// pending pairs after it, then an open pre for the benchmark command so each
// measured post has one to pair with. All of it is in the recent past, inside
// the store's ten-day pairing window and before every measured run.
func seedStore(s *sandbox, synced, pending int) storeFixture {
	s.b.Helper()
	hostname, _ := os.Hostname()
	base := time.Now().Add(-6 * time.Hour)
	var pre, post bytes.Buffer
	var cursor time.Time
	at := func(i int) time.Time { return base.Add(time.Duration(i) * 3 * time.Second) }

	for i := range synced + pending {
		c := storedCommand{
			Shell:     benchShell,
			SessionID: benchSession,
			Command:   seedCommands[i%len(seedCommands)],
			Hostname:  hostname,
			Username:  benchUser,
			Time:      at(i),
			PPID:      benchPPID,
		}
		pre.Write(storeLine(c, c.Time))

		end := c.Time.Add(800 * time.Millisecond)
		c.Phase, c.Time, c.EndTime, c.Result = 1, end, end, i%3
		post.Write(storeLine(c, end))
		if i == synced-1 {
			// The store re-reads records at the cursor (Before, not
			// !After), so it sits just past the last synced one.
			cursor = end.Add(time.Nanosecond)
		}
	}
	open := storedCommand{
		Shell: benchShell, SessionID: benchSession, Command: benchCommand,
		Hostname: hostname, Username: benchUser, Time: at(synced + pending), PPID: benchPPID,
	}
	pre.Write(storeLine(open, open.Time))

	if pre.Len() > maxFixtureBytes || post.Len() > maxFixtureBytes {
		s.b.Fatalf("fixture too large (pre %d B, post %d B, limit %d B)", pre.Len(), post.Len(), maxFixtureBytes)
	}
	f := storeFixture{s: s, postSize: int64(post.Len()), cursor: fmt.Appendf(nil, "\n%d\n", cursor.UnixNano())}
	for name, data := range map[string][]byte{"pre.txt": pre.Bytes(), "post.txt": post.Bytes(), "cursor.txt": f.cursor} {
		if err := os.WriteFile(s.path("commands", name), data, 0o644); err != nil {
			s.b.Fatal(err)
		}
	}
	return f
}

// check reports whether the last run appended to post.txt and moved the cursor.
func (f storeFixture) check() (postGrew, cursorMoved bool) {
	f.s.b.Helper()
	st, err := os.Stat(f.s.path("commands", "post.txt"))
	if err != nil {
		f.s.b.Fatal(err)
	}
	cursor, err := os.ReadFile(f.s.path("commands", "cursor.txt"))
	if err != nil {
		f.s.b.Fatal(err)
	}
	return st.Size() > f.postSize, !bytes.Equal(cursor, f.cursor)
}

// restore puts post.txt and cursor.txt back to their seeded state.
func (f storeFixture) restore() {
	f.s.b.Helper()
	if err := os.Truncate(f.s.path("commands", "post.txt"), f.postSize); err != nil {
		f.s.b.Fatal(err)
	}
	if err := os.WriteFile(f.s.path("commands", "cursor.txt"), f.cursor, 0o644); err != nil {
		f.s.b.Fatal(err)
	}
}

// countLines counts newline-terminated lines in the file at path.
func countLines(b *testing.B, path string) int {
	b.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		b.Fatal(err)
	}
	return bytes.Count(data, []byte{'\n'})
}
