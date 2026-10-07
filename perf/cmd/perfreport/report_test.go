package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/report.golden.md")

// testdata/{base,head}.txt cover every row status: daemon/pre is 20% slower,
// daemon/post 6% slower (under the threshold), direct/pre 25% faster,
// direct/post-sync 50% slower but by only 0.1 ms, direct/post failed on head
// and direct/new exists only on head.
func TestReportGolden(t *testing.T) {
	var stdout bytes.Buffer
	md := filepath.Join(t.TempDir(), "report.md")
	err := run([]string{
		"-base", "testdata/base.txt", "-head", "testdata/head.txt",
		"-base-label", "`main` @ abc1234", "-head-label", "#42 @ def5678",
		"-md", md, "-github", "-note", "Go go1.27.1.",
	}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(md)
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "report.golden.md")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("report differs from %s (run with -update to accept):\n%s", golden, got)
	}

	wantAnnotations := "::warning title=shelltime track perf::Track/daemon/pre is 20% slower (5.99 ms → 7.19 ms, p=0.000)\n" +
		"::warning title=shelltime track perf::Track/direct/post has no result on head; its benchmark failed (see the job log)\n"
	if stdout.String() != wantAnnotations {
		t.Errorf("annotations:\n%s\nwant:\n%s", stdout.String(), wantAnnotations)
	}
}

func TestGithubOutputs(t *testing.T) {
	out := filepath.Join(t.TempDir(), "output")
	t.Setenv("GITHUB_OUTPUT", out)
	md := filepath.Join(t.TempDir(), "report.md")

	if err := run([]string{"-base", "testdata/base.txt", "-head", "testdata/head.txt", "-md", md, "-github"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-skipped", "Binary unchanged.", "-md", md, "-github"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := "regression=true\nskipped=false\nregression=false\nskipped=true\n"
	if string(got) != want {
		t.Errorf("GITHUB_OUTPUT = %q, want %q", got, want)
	}
	report, _ := os.ReadFile(md)
	if !strings.HasPrefix(string(report), marker+"\n") || !strings.Contains(string(report), "⏭️ Binary unchanged.") {
		t.Errorf("skipped report = %q", report)
	}
}

func TestRequiresInputs(t *testing.T) {
	if err := run([]string{"-base", "testdata/base.txt"}, &bytes.Buffer{}); err == nil {
		t.Error("want an error without -head")
	}
}

func TestGate(t *testing.T) {
	o := options{threshold: 0.10, minDelta: 250 * time.Microsecond}
	series := func(center float64) []float64 {
		vs := make([]float64, 10)
		for i := range vs {
			vs[i] = center * (1 + float64(i-5)/1000)
		}
		return vs
	}
	tests := []struct {
		name       string
		base, head []float64
		want       status
	}{
		{"slower", series(5e-3), series(6e-3), statusSlower},
		{"faster", series(6e-3), series(5e-3), statusFaster},
		{"under threshold", series(5e-3), series(5.4e-3), statusSame},
		{"under min delta", series(200e-6), series(400e-6), statusSame},
		{"not significant", series(5e-3)[:2], series(6e-3)[:2], statusSame},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gate(newMetric(tt.base, tt.head), o); got != tt.want {
				t.Errorf("gate = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNoisyFloor(t *testing.T) {
	base := &samples{order: []string{floorName}, values: map[string]map[string][]float64{
		floorName: {wallUnit: {1.00e-3, 1.01e-3, 0.99e-3, 1.02e-3, 1.00e-3, 0.98e-3}},
	}}
	head := &samples{order: []string{floorName}, values: map[string]map[string][]float64{
		floorName: {wallUnit: {1.20e-3, 1.21e-3, 1.19e-3, 1.22e-3, 1.20e-3, 1.18e-3}},
	}}
	res := compare(base, head, options{threshold: 0.10, minDelta: 250 * time.Microsecond, noise: 0.05})
	if !res.noisy || res.regressions != 0 {
		t.Fatalf("noisy = %v, regressions = %d; want a noisy run and no regression from the floor", res.noisy, res.regressions)
	}
	if h := res.headline(options{threshold: 0.10}); !strings.Contains(h, "noisy") {
		t.Errorf("headline %q does not mention noise", h)
	}
}
