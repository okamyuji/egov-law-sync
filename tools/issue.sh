#!/usr/bin/env bash
# issue: 終了コードとruns/のJSONからラベルを決め、Issueを作るかコメントする。
# 使い方: issue.sh <exit-code> <runs-dir> [job-status]
set -eu

# 強制終了でCLIが終了コードを書けなかった場合は空で渡ってくる
code="${1:-killed}"
# runs/はcommit済みなので、目印より古いJSONを拾うと前回の実行の内容でIssueを書いてしまう。
# 目印はワークフローがCLIを起動する直前に作る。目印が無い場合はbuildより前で落ちているので何も選ばない
run_json=""
if [ -f bin/run-start ]; then
  run_json=$(find "${2:-/nonexistent}" -name '*.json' -newer bin/run-start 2>/dev/null | sort | tail -1)
fi
# CLIが0で終わった後にrelease/commit/pushが失敗した場合もanomalyにする
if [ "${3:-success}" = "failure" ] && [ "$code" = "0" ]; then
  code="0 (job failed after CLI)"
fi

if [ "$code" != "0" ]; then
  label="anomaly"
elif [ -s "$run_json" ] && [ "$(jq -r '(.warnings // []) | length' "$run_json")" != "0" ]; then
  label="warning"
else
  exit 0
fi

# 今回のJSONが無い場合（CLIがrunを書く前に落ちた場合、make buildが失敗した場合）も空扱いでanomaly Issueは出す
if [ -s "$run_json" ]; then
  json=$(cat "$run_json")
else
  json='{}'
fi
kind=$(jq -r '.kind // "unknown"' <<<"$json")
date_jst=$(jq -r '.date_jst // ""' <<<"$json")
counts=$(jq -r '.counts // {} | to_entries | map("\(.key): \(.value)") | join(", ")' <<<"$json")
anomalies=$(jq -r '(.anomalies // []) | if length == 0 then "(なし)" else map("- \(.)") | join("\n") end' <<<"$json")
warnings=$(jq -r '(.warnings // []) | if length == 0 then "(なし)" else map("- \(.)") | join("\n") end' <<<"$json")
change_count=$(jq -r '(.changes // []) | length' <<<"$json")
changes=$(jq -r '(.changes // [])[0:200] | if length == 0 then "(なし)" else map("- \(.Kind) \(.LawID) \(.OldRevision) -> \(.NewRevision)") | join("\n") end' <<<"$json")

title="[$label] $kind $date_jst (exit $code)"
body=$(printf 'counts: %s\n\nanomalies:\n%s\n\nwarnings:\n%s\n\nchanges (%s件のうち先頭200件):\n%s\n' \
  "$counts" "$anomalies" "$warnings" "$change_count" "$changes")

existing=$(gh issue list --label "$label" --state open --json number --jq '.[0].number // empty' 2>/dev/null || true)
if [ -n "$existing" ]; then
  gh issue comment "$existing" --body "$title"$'\n\n'"$body"
else
  gh issue create --title "$title" --body "$body" --label "$label"
fi
