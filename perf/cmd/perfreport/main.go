// Command perfreport compares two runs of the perf benchmarks (base and head)
// and writes a markdown report. It never fails on a regression: it warns.
//
//	perfreport -base a.txt -head b.txt -md report.md [-github]
//	perfreport -skipped "reason" -md report.md
//
// With -github it also prints ::warning annotations for flagged scenarios and
// writes regression=true|false and skipped=true|false to $GITHUB_OUTPUT.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "perfreport:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("perfreport", flag.ContinueOnError)
	var (
		basePath   = fs.String("base", "", "benchmark output of the base binary")
		headPath   = fs.String("head", "", "benchmark output of the head binary")
		baseBin    = fs.String("base-bin", "", "base binary, for its size")
		headBin    = fs.String("head-bin", "", "head binary, for its size")
		benchstat  = fs.String("benchstat", "", "file with `benchstat` output to embed")
		mdPath     = fs.String("md", "", "write the markdown report here (default stdout)")
		skipped    = fs.String("skipped", "", "write a report that only says why the comparison was skipped")
		github     = fs.Bool("github", false, "emit GitHub Actions annotations and step outputs")
		o          options
		thresholdP float64
	)
	fs.Float64Var(&thresholdP, "threshold", 10, "flag scenarios more than this many percent slower")
	fs.DurationVar(&o.minDelta, "min-delta", 250*time.Microsecond, "and more than this much slower in absolute terms")
	fs.StringVar(&o.baseLabel, "base-label", "base", "how the report names the base")
	fs.StringVar(&o.headLabel, "head-label", "head", "how the report names the head")
	fs.StringVar(&o.note, "note", "", "extra footer text")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o.threshold = thresholdP / 100
	o.noise = 0.05

	var md string
	regression := false
	if *skipped != "" {
		md = skippedMarkdown(*skipped)
	} else {
		if *basePath == "" || *headPath == "" {
			return fmt.Errorf("-base and -head are required")
		}
		base, err := parseFile(*basePath)
		if err != nil {
			return err
		}
		head, err := parseFile(*headPath)
		if err != nil {
			return err
		}
		o.baseSize, o.headSize = fileSize(*baseBin), fileSize(*headBin)
		if *benchstat != "" {
			data, err := os.ReadFile(*benchstat)
			if err != nil {
				return err
			}
			o.benchstat = string(data)
		}
		res := compare(base, head, o)
		md = res.markdown(o)
		regression = res.regressions > 0 || res.failed > 0
		if *github {
			for _, a := range res.annotations() {
				fmt.Fprintln(stdout, a)
			}
		}
	}

	if *mdPath == "" {
		fmt.Fprint(stdout, md)
	} else if err := os.WriteFile(*mdPath, []byte(md), 0o644); err != nil {
		return err
	}
	if *github {
		return writeOutputs(map[string]bool{"regression": regression, "skipped": *skipped != ""})
	}
	return nil
}

func parseFile(path string) (*samples, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parse(f, path)
}

func fileSize(path string) int64 {
	if path == "" {
		return 0
	}
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// writeOutputs appends step outputs to $GITHUB_OUTPUT when it is set.
func writeOutputs(outputs map[string]bool) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, k := range []string{"regression", "skipped"} {
		if _, err := fmt.Fprintf(f, "%s=%t\n", k, outputs[k]); err != nil {
			return err
		}
	}
	return nil
}
