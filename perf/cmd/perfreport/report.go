package main

import (
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/perf/benchfmt"
	"golang.org/x/perf/benchmath"
	"golang.org/x/perf/benchunit"
)

// marker is the first line of every report; comment.sh finds the PR comment
// to update by it.
const marker = "<!-- shelltime-track-perf -->"

const (
	// wallUnit is the only unit that can flag a regression: wall time is
	// what the user waits for at the prompt.
	wallUnit = "sec/op"
	// floorName is the fork/exec baseline, which does not depend on the
	// binary under test.
	floorName = "Startup/floor"
)

// extraUnits are shown in the details table, in this order.
var extraUnits = []struct{ unit, label string }{
	{"p95-sec/op", "p95 wall"},
	{"user-sec/op", "user CPU"},
	{"sys-sec/op", "sys CPU"},
	{"peak-rss-B", "peak RSS"},
}

type options struct {
	threshold float64       // relative slowdown that flags a regression, e.g. 0.10
	minDelta  time.Duration // and the absolute slowdown it must also exceed
	noise     float64       // floor drift past which the run is called noisy

	baseLabel, headLabel string
	baseSize, headSize   int64  // binary sizes in bytes, 0 if unknown
	benchstat            string // raw `benchstat` output, optional
	note                 string // extra footer line, optional
}

// samples holds every value of every benchmark in one results file.
type samples struct {
	order  []string                        // benchmark names, first-seen order
	values map[string]map[string][]float64 // name → unit → values
	config map[string]string               // goos, goarch, cpu, …
}

var procsSuffix = regexp.MustCompile(`-\d+$`)

// parse reads Go benchmark output. Names lose the "Benchmark" prefix (benchfmt
// drops it) and the -GOMAXPROCS suffix, and units are tidied (ns/op is read as
// sec/op).
func parse(r io.Reader, fileName string) (*samples, error) {
	s := &samples{values: map[string]map[string][]float64{}, config: map[string]string{}}
	br := benchfmt.NewReader(r, fileName)
	for br.Scan() {
		switch rec := br.Result().(type) {
		case *benchfmt.Result:
			name := procsSuffix.ReplaceAllString(rec.Name.String(), "")
			units, ok := s.values[name]
			if !ok {
				units = map[string][]float64{}
				s.values[name] = units
				s.order = append(s.order, name)
			}
			for _, v := range rec.Values {
				// The reader leaves zero values untidied ("0 sys-ns/op").
				val, unit := benchunit.Tidy(v.Value, v.Unit)
				units[unit] = append(units[unit], val)
			}
			for _, c := range rec.Config {
				if _, seen := s.config[c.Key]; !seen && c.File {
					s.config[c.Key] = string(c.Value)
				}
			}
		case *benchfmt.SyntaxError:
			return nil, rec
		}
	}
	return s, br.Err()
}

type status int

const (
	statusSame status = iota
	statusSlower
	statusFaster
	statusNew    // no base result: a scenario added on head
	statusFailed // no head result: the scenario failed or was removed
	statusFloor  // the A/A baseline row
)

// metric compares one unit of one benchmark.
type metric struct {
	ok         bool // both sides have values
	base, head benchmath.Summary
	cmp        benchmath.Comparison
}

func (m metric) significant() bool { return m.ok && m.cmp.P < m.cmp.Alpha }

func (m metric) delta() float64 {
	if !m.ok || m.base.Center == 0 {
		return 0
	}
	return m.head.Center/m.base.Center - 1
}

type row struct {
	name   string
	status status
	wall   metric
	extra  map[string]metric
	// lone holds the summary of the only side that has results, for new and
	// failed rows.
	lone *benchmath.Summary
}

type result struct {
	rows                     []row
	regressions, faster, new int
	failed                   int
	noisy                    bool
	runs                     int // samples per side
	config                   map[string]string
}

func newMetric(base, head []float64) metric {
	if len(base) == 0 || len(head) == 0 {
		return metric{}
	}
	thr := &benchmath.DefaultThresholds
	// NewSample sorts its input in place.
	sb := benchmath.NewSample(slices.Clone(base), thr)
	sh := benchmath.NewSample(slices.Clone(head), thr)
	return metric{
		ok:   true,
		base: benchmath.AssumeNothing.Summary(sb, 0.95),
		head: benchmath.AssumeNothing.Summary(sh, 0.95),
		cmp:  benchmath.AssumeNothing.Compare(sb, sh),
	}
}

func compare(base, head *samples, o options) result {
	res := result{config: head.config}
	if len(res.config) == 0 {
		res.config = base.config
	}
	names := slices.Clone(head.order)
	for _, n := range base.order {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	for _, name := range names {
		b, h := base.values[name], head.values[name]
		r := row{name: name, wall: newMetric(b[wallUnit], h[wallUnit]), extra: map[string]metric{}}
		for _, u := range extraUnits {
			r.extra[u.unit] = newMetric(b[u.unit], h[u.unit])
		}
		res.runs = max(res.runs, len(h[wallUnit]), len(b[wallUnit]))

		switch {
		case len(h[wallUnit]) == 0:
			r.status = statusFailed
			r.lone = loneSummary(b[wallUnit])
			res.failed++
		case len(b[wallUnit]) == 0:
			r.status = statusNew
			r.lone = loneSummary(h[wallUnit])
			res.new++
		case name == floorName:
			r.status = statusFloor
			res.noisy = r.wall.significant() && math.Abs(r.wall.delta()) > o.noise
		default:
			r.status = gate(r.wall, o)
			switch r.status {
			case statusSlower:
				res.regressions++
			case statusFaster:
				res.faster++
			}
		}
		res.rows = append(res.rows, r)
	}
	return res
}

// gate flags a change only when it is statistically significant, larger than
// the relative threshold and larger than the absolute floor. The last guard
// stops sub-millisecond scenarios from flagging on scheduler jitter.
func gate(m metric, o options) status {
	if !m.significant() {
		return statusSame
	}
	d := m.delta()
	abs := time.Duration(math.Abs(m.head.Center-m.base.Center) * float64(time.Second))
	switch {
	case d > o.threshold && abs > o.minDelta:
		return statusSlower
	case d < -o.threshold && abs > o.minDelta:
		return statusFaster
	}
	return statusSame
}

func loneSummary(values []float64) *benchmath.Summary {
	if len(values) == 0 {
		return nil
	}
	s := benchmath.AssumeNothing.Summary(benchmath.NewSample(slices.Clone(values), &benchmath.DefaultThresholds), 0.95)
	return &s
}

// headline is the one-line verdict at the top of the report.
func (res result) headline(o options) string {
	var parts []string
	switch {
	case res.regressions > 0:
		parts = append(parts, fmt.Sprintf("⚠️ **%s got slower**: more than %s in wall time", plural(res.regressions, "scenario"), pct(o.threshold)))
	case res.faster > 0:
		parts = append(parts, fmt.Sprintf("🚀 **%s got faster** by more than %s", plural(res.faster, "scenario"), pct(o.threshold)))
	default:
		parts = append(parts, "✅ **No significant change** in `shelltime track` latency")
	}
	if res.failed > 0 {
		parts = append(parts, fmt.Sprintf("❌ %s produced no result on head", plural(res.failed, "scenario")))
	}
	if res.noisy {
		parts = append(parts, "🎲 the runner was noisy (the fork/exec floor moved too), so treat small changes with care")
	}
	return strings.Join(parts, " · ")
}

// markdown renders the report.
func (res result) markdown(o options) string {
	var sb strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&sb, format, args...) }

	w("%s\n## ⏱️ `shelltime track` performance\n\n", marker)
	w("%s\n\n", res.headline(o))
	w("Comparing %s → %s. Each scenario execs the binary the way the shell hooks do; values are the median of %d interleaved rounds. Lower is better.\n\n",
		o.baseLabel, o.headLabel, res.runs)

	w("| Scenario | base | head | Δ | p | |\n|---|--:|--:|--:|--:|:-:|\n")
	for _, r := range res.rows {
		switch r.status {
		case statusNew:
			w("| `%s` | — | %s | new | | 🆕 |\n", r.name, summaryCell(wallUnit, r.lone))
		case statusFailed:
			w("| `%s` | %s | — | | | ❌ |\n", r.name, summaryCell(wallUnit, r.lone))
		default:
			w("| `%s` | %s | %s | %s | %s | %s |\n", r.name,
				summaryCell(wallUnit, &r.wall.base), summaryCell(wallUnit, &r.wall.head),
				deltaCell(r.wall), fmt.Sprintf("%.3f", r.wall.cmp.P), statusCell(r.status, res.noisy))
		}
	}
	if o.baseSize > 0 && o.headSize > 0 {
		d := float64(o.headSize)/float64(o.baseSize) - 1
		w("\nBinary size: %s → %s (%+.1f%%).\n", fmtBytes(float64(o.baseSize)), fmtBytes(float64(o.headSize)), 100*d)
	}

	w("\n<details><summary>CPU, tail latency and memory</summary>\n\n")
	w("Per exec, base → head (Δ when significant).\n\n| Scenario |")
	for _, u := range extraUnits {
		w(" %s |", u.label)
	}
	w("\n|---|")
	for range extraUnits {
		w("--:|")
	}
	w("\n")
	for _, r := range res.rows {
		w("| `%s` |", r.name)
		for _, u := range extraUnits {
			m := r.extra[u.unit]
			if !m.ok {
				w(" — |")
				continue
			}
			cell := fmtValue(u.unit, m.base.Center) + " → " + fmtValue(u.unit, m.head.Center)
			if d := deltaCell(m); d != "~" {
				cell += " (" + d + ")"
			}
			w(" %s |", cell)
		}
		w("\n")
	}
	w("\n</details>\n")

	if o.benchstat != "" {
		w("\n<details><summary>benchstat</summary>\n\n```\n%s\n```\n\n</details>\n", strings.TrimRight(o.benchstat, "\n"))
	}

	w("\n<sub>Δ compares medians; ~ means no significant difference (Mann-Whitney U, p ≥ 0.05). "+
		"A scenario is flagged when it is more than %s and more than %s slower with p < 0.05. "+
		"`Startup/floor` runs `true`, not shelltime: it is an A/A check of runner noise.",
		pct(o.threshold), o.minDelta)
	if env := runnerInfo(res.config); env != "" {
		w(" Runner: %s.", env)
	}
	if o.note != "" {
		w(" %s", o.note)
	}
	w("</sub>\n")
	return sb.String()
}

// annotations are GitHub workflow commands, one per flagged scenario.
func (res result) annotations() []string {
	var out []string
	for _, r := range res.rows {
		switch r.status {
		case statusSlower:
			out = append(out, fmt.Sprintf("::warning title=shelltime track perf::%s is %s slower (%s → %s, p=%.3f)",
				r.name, pct(r.wall.delta()), fmtSec(r.wall.base.Center), fmtSec(r.wall.head.Center), r.wall.cmp.P))
		case statusFailed:
			out = append(out, fmt.Sprintf("::warning title=shelltime track perf::%s has no result on head; its benchmark failed (see the job log)", r.name))
		}
	}
	return out
}

func skippedMarkdown(reason string) string {
	return fmt.Sprintf("%s\n## ⏱️ `shelltime track` performance\n\n⏭️ %s\n", marker, reason)
}

func summaryCell(unit string, s *benchmath.Summary) string {
	if s == nil {
		return "—"
	}
	return fmtValue(unit, s.Center) + " ±" + s.PctRangeString()
}

func deltaCell(m metric) string {
	if !m.ok {
		return ""
	}
	return m.cmp.FormatDelta(m.base.Center, m.head.Center)
}

func statusCell(s status, noisy bool) string {
	switch s {
	case statusSlower:
		return "⚠️"
	case statusFaster:
		return "🚀"
	case statusFloor:
		if noisy {
			return "🎲"
		}
		return "A/A"
	}
	return "✅"
}

func fmtValue(unit string, v float64) string {
	switch {
	case strings.HasSuffix(unit, "sec/op"):
		return fmtSec(v)
	case benchunit.ClassOf(unit) == benchunit.Binary:
		return fmtBytes(v)
	}
	return fmt.Sprintf("%.3g", v)
}

func fmtSec(v float64) string {
	switch {
	case v >= 1:
		return fmt.Sprintf("%.2f s", v)
	case v >= 1e-3:
		return fmt.Sprintf("%.2f ms", v*1e3)
	}
	return fmt.Sprintf("%.0f µs", v*1e6)
}

func fmtBytes(v float64) string {
	return fmt.Sprintf("%.1f MiB", v/(1<<20))
}

func pct(f float64) string {
	return fmt.Sprintf("%.0f%%", 100*math.Abs(f))
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func runnerInfo(config map[string]string) string {
	var parts []string
	if config["goos"] != "" {
		parts = append(parts, config["goos"]+"/"+config["goarch"])
	}
	if config["cpu"] != "" {
		parts = append(parts, config["cpu"])
	}
	return strings.Join(parts, ", ")
}
