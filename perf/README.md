# `shelltime track` performance tests

The shell hooks (`model/hooks/`) run `shelltime track` in the foreground before
and after **every** command, so its latency sits directly between the user and
their next prompt. The benchmarks here exec the real binary, with the same flags
the zsh hook passes, in an isolated `$HOME`. That measures what users feel:
exec, Go runtime and package init, `main`'s config read and telemetry setup,
then the track logic.

This directory is its own Go module, so `golang.org/x/perf` never takes part in
version selection for the shipped binaries, and the root `go test ./...` skips
it.

## Scenarios

| Benchmark | What it measures |
|---|---|
| `Startup/floor` | Runs `true`, not shelltime: the runner's fork/exec cost. Base and head run the same thing, so this row is an A/A check of runner noise |
| `Startup/version` | `shelltime --version`: process start, init, config read, no command logic |
| `Track/daemon/pre`, `/post` | The default install. A fake daemon listens on `/tmp/shelltime.sock`; track sends it the event and returns |
| `Track/direct/pre` | No daemon. track appends to `~/.shelltime/commands/pre.txt` |
| `Track/direct/post` | No daemon, below the flush threshold. track appends to `post.txt` and reads the whole store (500 synced pairs) to decide whether to sync |
| `Track/direct/post-sync` | The post that reaches `flushCount` and sends the batch to a local fake API (once every 10 commands in direct mode) |

Every scenario checks that the binary really took its path: the fake daemon got
one event per run, the store grew, the API got one batch per run, and
`log.log` holds no errors. `track` always exits 0, so without these checks a
broken run would just look fast. A failed check fails that scenario only, and
the report shows it as ❌.

Besides wall time (`sec/op`), each scenario reports per-exec user and sys CPU,
p50/p95 wall time and peak RSS of the child process.

On Linux the direct post path execs `lsb_release`. On Ubuntu that is a Python
script that adds tens of noisy milliseconds, so the harness puts a shell stub
with the same output first in `PATH`. Set `SHELLTIME_BENCH_REAL_OSINFO=1` to
measure the real one.

## Running locally

```sh
# Benchmark the current tree (builds ./cmd/cli once)
go -C perf test -run '^$' -bench . -benchtime 50x -count 5

# Benchmark a specific binary
SHELLTIME_BENCH_BIN=/path/to/shelltime go -C perf test -run '^$' -bench . -benchtime 50x

# Reproduce the CI comparison: base commit vs the working tree, uncommitted changes included
perf/compare.sh origin/main
ROUNDS=4 ITERS=30 perf/compare.sh HEAD~1 HEAD /tmp/perf   # quicker, explicit head and output dir
```

The `Track/*` scenarios are skipped while a real shelltime daemon owns
`/tmp/shelltime.sock`, which is the usual state of a developer machine: the
daemon would receive the fake commands. Stop the daemon to run them. The socket
path is fixed in the CLI, so it cannot be redirected.

## In CI

`.github/workflows/perf.yaml` runs on every pull request to `main`, every push
to `main` and on demand. It compares:

- a pull request against the commit it is merged onto (`HEAD^1` of the merge commit),
- a push to `main` against the previous `main` tip (`github.event.before`).

`compare.sh` builds both binaries with identical flags (`CGO_ENABLED=0
-trimpath -buildvcs=false -ldflags "-s -w"`). If they come out byte-identical,
for example on a docs-only change, it skips the run, unless the harness itself
changed. Otherwise it compiles the harness once from head and runs it against
both binaries in 10 rounds of 100 execs per scenario. The rounds alternate in
ABBA order on the same runner, so drift and noise hit both sides alike.

The report goes to the job summary and, on pull requests from this repository,
to a single comment that later runs update. Fork pull requests get a read-only
token, so they only get the summary.

### Reading the report

Values are medians across rounds, with a 95% confidence interval. A scenario is
flagged ⚠️ when it is **more than 10% slower, more than 0.25 ms slower and
significant (Mann-Whitney U, p < 0.05)**. The absolute floor keeps
sub-millisecond scenarios from flagging on scheduler jitter. ~ means no
significant difference. 🚀 is the same rule in the other direction.

If `Startup/floor` itself moved significantly, the headline says the runner was
noisy. Treat small changes in that run with care.

A regression never fails the check. It shows up as ⚠️ in the report and as a
warning annotation on the run. Rerun the job if you suspect noise, or reproduce
it locally with `compare.sh`. To tune the gate, change `THRESHOLD` in the
workflow, or `-min-delta` in `cmd/perfreport`.
