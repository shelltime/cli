#!/usr/bin/env bash
# Create or update the pull request comment that holds the perf report.
#
#   perf/comment.sh <pr-number> <report.md> [--update-only]
#
# The comment is found by the report's first line, a hidden marker, among the
# comments github-actions posted. With --update-only an existing comment is
# refreshed but no new one is created. Needs GH_TOKEN and GITHUB_REPOSITORY,
# which Actions provides.
set -euo pipefail

usage="usage: perf/comment.sh <pr-number> <report.md> [--update-only]"
pr=${1:?$usage}
report=${2:?$usage}
mode=${3:-}
repo=${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}

marker=$(head -n 1 "$report")
id=$(gh api --paginate "repos/$repo/issues/$pr/comments" \
	--jq ".[] | select(.user.login == \"github-actions[bot]\" and (.body | startswith(\"$marker\"))) | .id" |
	tail -n 1)

if [[ -n $id ]]; then
	gh api --method PATCH "repos/$repo/issues/comments/$id" -F body=@"$report" >/dev/null
	echo "updated comment $id"
elif [[ $mode != --update-only ]]; then
	gh api --method POST "repos/$repo/issues/$pr/comments" -F body=@"$report" >/dev/null
	echo "posted a new comment"
fi
