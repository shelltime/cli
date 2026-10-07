#!/usr/bin/env bash
# Compare `shelltime track` latency between two commits, the way CI does.
#
#   perf/compare.sh <base-ref> [head-ref|WORKTREE] [outdir]
#
# Builds both binaries with identical flags, runs one harness (from this
# checkout) against both in interleaved rounds on the same machine, and writes
# <outdir>/report.md. The head defaults to WORKTREE: the checkout as it is,
# uncommitted changes included.
#
# Environment:
#   ROUNDS      interleaved rounds per binary (10)
#   ITERS       execs per scenario per round (100)
#   THRESHOLD   percent slowdown that flags a scenario (10)
#   FORCE=1     compare even when the two binaries are byte-identical
#   BASE_LABEL, HEAD_LABEL   how the report names the two sides
set -euo pipefail

usage="usage: perf/compare.sh <base-ref> [head-ref|WORKTREE] [outdir]"
base_ref=${1:?$usage}
head_ref=${2:-WORKTREE}
root=$(git rev-parse --show-toplevel)
out=${3:-$(mktemp -d "${TMPDIR:-/tmp}/shelltime-perf.XXXXXX")}
rounds=${ROUNDS:-10}
iters=${ITERS:-100}

mkdir -p "$out"
out=$(cd "$out" && pwd)
rm -rf "$out/a" "$out/b"
rm -f "$out"/{a.txt,b.txt,round.txt,benchstat.txt,base-build.log,report.md,perf.test}

scratch=$(mktemp -d "${TMPDIR:-/tmp}/shelltime-perf-src.XXXXXX")
cleanup() {
	for wt in "$scratch"/*/; do
		[[ -d $wt ]] && git -C "$root" worktree remove --force "$wt" >/dev/null 2>&1 || true
	done
	rm -rf "$scratch"
	git -C "$root" worktree prune
}
trap cleanup EXIT

# checkout <ref> <name>: a detached worktree of ref, printed as a path.
checkout() {
	local sha
	sha=$(git -C "$root" rev-parse --verify "$1^{commit}")
	git -C "$root" worktree add --detach --quiet "$scratch/$2" "$sha"
	echo "$scratch/$2"
}

# build <src> <binary>: the CLI with release-like flags. -trimpath and
# -buildvcs=false keep the checkout path and VCS stamp out of the binary, so
# identical code gives byte-identical binaries.
build() {
	(cd "$1" && CGO_ENABLED=0 go build -trimpath -buildvcs=false \
		-ldflags "-s -w -X main.version=perf" -o "$2" ./cmd/cli)
}

report() {
	go -C "$root/perf" run ./cmd/perfreport -md "$out/report.md" ${GITHUB_ACTIONS:+-github} "$@"
}

base_sha=$(git -C "$root" rev-parse --short=7 --verify "$base_ref^{commit}")
if [[ $head_ref == WORKTREE ]]; then
	head_src=$root
	head_desc="the working tree"
else
	head_src=$(checkout "$head_ref" head)
	head_desc=$(git -C "$root" rev-parse --short=7 --verify "$head_ref^{commit}")
fi
base_label=${BASE_LABEL:-\`$base_sha\`}
head_label=${HEAD_LABEL:-$head_desc}

echo "building head ($head_desc) and base ($base_sha)"
build "$head_src" "$out/b/shelltime"
base_src=$(checkout "$base_ref" base)
if ! build "$base_src" "$out/a/shelltime" 2>"$out/base-build.log"; then
	cat "$out/base-build.log" >&2
	report -skipped "The base commit $base_label does not build, so there is nothing to compare against."
	echo "report: $out/report.md"
	exit 0
fi

if [[ ${FORCE:-0} != 1 ]] && cmp -s "$out/a/shelltime" "$out/b/shelltime"; then
	report -skipped "The \`shelltime\` binary is byte-identical to $base_label, so its performance cannot have changed."
	echo "report: $out/report.md"
	exit 0
fi

# One harness, built from this checkout, measures both binaries.
go -C "$root/perf" test -c -o "$out/perf.test" .

for ((i = 1; i <= rounds; i++)); do
	# ABBA order, so neither side always runs first.
	if ((i % 2)); then order="a b"; else order="b a"; fi
	for side in $order; do
		echo "round $i/$rounds: $([[ $side == a ]] && echo base || echo head)"
		if ! SHELLTIME_BENCH_BIN="$out/$side/shelltime" "$out/perf.test" \
			-test.run='^$' -test.bench=. -test.benchtime="${iters}x" -test.count=1 \
			-test.timeout=20m >"$out/round.txt" 2>&1; then
			# A failed scenario only loses its own result; show why.
			grep -v '^Benchmark' "$out/round.txt" >&2 || true
		fi
		cat "$out/round.txt" >>"$out/$side.txt"
	done
done
rm -f "$out/round.txt"

(cd "$root/perf" && go tool benchstat base="$out/a.txt" head="$out/b.txt") >"$out/benchstat.txt" 2>&1 || true

report -base "$out/a.txt" -head "$out/b.txt" \
	-base-bin "$out/a/shelltime" -head-bin "$out/b/shelltime" \
	-benchstat "$out/benchstat.txt" -threshold "${THRESHOLD:-10}" \
	-base-label "$base_label" -head-label "$head_label" \
	-note "$(go env GOVERSION), $rounds rounds × $iters execs per scenario and binary."
echo "report: $out/report.md"
