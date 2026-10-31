#!/usr/bin/env bash
# The GitHub Action's step: checks the issue or pull request that started the
# workflow, or PHANTOM_FILE, and comments with what phantom found.
set -euo pipefail

phantom="$RUNNER_TEMP/phantom"
text="$RUNNER_TEMP/phantom-text.md"
json="$RUNNER_TEMP/phantom.json"
report="$RUNNER_TEMP/phantom-report.md"

case "$PHANTOM_COMMENT" in
missing | doubtful | always | never) ;;
*)
  echo "::error::phantom: comment must be missing, doubtful, always or never, not $PHANTOM_COMMENT"
  exit 2
  ;;
esac

number=""
if [ -n "$PHANTOM_FILE" ]; then
  cp -- "$PHANTOM_FILE" "$text"
else
  # The text is read from the event file, never pasted into a command, so
  # nothing a writer puts in an issue can run here.
  number=$(jq -r '(.issue // .pull_request // {}).number // empty' "$GITHUB_EVENT_PATH")
  if [ -z "$number" ]; then
    echo "::notice::phantom: this event has no issue or pull request to check"
    exit 0
  fi
  jq -r '(.issue // .pull_request) | [("# " + (.title // "")), (.body // "")] | join("\n\n")' "$GITHUB_EVENT_PATH" >"$text"
fi

status=0
"$phantom" -repo "$PHANTOM_REPO" -format json "$text" >"$json" || status=$?
if [ "$status" -gt 1 ]; then
  exit "$status"
fi
"$phantom" -repo "$PHANTOM_REPO" -format markdown "$text" >"$report" || true

mentioned=$(jq .mentioned "$json")
missing=$(jq .missing "$json")
doubtful=$(jq .doubtful "$json")
{
  echo "mentioned=$mentioned"
  echo "missing=$missing"
  echo "doubtful=$doubtful"
  echo "report=$report"
} >>"$GITHUB_OUTPUT"
cat "$report" >>"$GITHUB_STEP_SUMMARY"
echo "phantom: $mentioned mentioned, $missing missing, $doubtful doubtful"

if [ -n "$number" ] && [ "$PHANTOM_COMMENT" != never ]; then
  marker="<!-- phantom -->"
  comments="repos/$GITHUB_REPOSITORY/issues/$number/comments"
  body="$RUNNER_TEMP/phantom-comment.md"
  {
    echo "$marker"
    cat "$report"
  } >"$body"
  earlier=$(gh api --paginate "$comments" --jq ".[] | select(.body | startswith(\"$marker\")) | .id" | tail -n 1) || earlier=""
  wanted=false
  case "$PHANTOM_COMMENT" in
  always) wanted=true ;;
  missing) [ "$missing" -eq 0 ] || wanted=true ;;
  doubtful) [ $((missing + doubtful)) -eq 0 ] || wanted=true ;;
  esac
  if [ -n "$earlier" ]; then
    gh api -X PATCH "repos/$GITHUB_REPOSITORY/issues/comments/$earlier" -F body=@"$body" >/dev/null ||
      echo "::warning::phantom could not update its comment; the report is in the job summary"
  elif [ "$wanted" = true ]; then
    # A pull request from a fork gets a token that cannot comment.
    gh api "$comments" -F body=@"$body" >/dev/null ||
      echo "::warning::phantom could not comment; the report is in the job summary"
  fi
fi

if [ "$PHANTOM_FAIL" = true ] && [ "$missing" -gt 0 ]; then
  echo "::error::phantom: $missing of the $mentioned pieces of code mentioned are not in the repository"
  exit 1
fi
