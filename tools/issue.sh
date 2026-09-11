#!/usr/bin/env bash
# issue: 終了コードとruns/のJSONからラベルを決め、Issueを作るかコメントする。
# 使い方: issue.sh <exit-code> <runs-dir> [job-status]
set -eu

# 強制終了でCLIが終了コードを書けなかった場合は空で渡ってくる
code="${1:-killed}"
# ファイル名はUTC時刻なので辞書順の最後が最新
run_json=$(find "${2:-/nonexistent}" -name '*.json' 2>/dev/null | sort | tail -1)
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

# runs/にJSONが無い場合（CLIがrunを書く前に落ちた場合）も空扱いでanomaly Issueは出す
json=$(cat "$run_json" 2>/dev/null || echo '{}')
kind=$(jq -r '.kind // "unknown"' <<<"$json")
date_jst=$(jq -r '.date_jst // ""' <<<"$json")
counts=$(jq -r '.counts // {} | to_entries | map("\(.key): \(.value)") | join(", ")' <<<"$json")
warnings=$(jq -r '(.warnings // []) | map("- \(.)") | join("\n")' <<<"$json")

title="[$label] $kind $date_jst (exit $code)"
body=$(printf 'counts: %s\n\nwarnings:\n%s\n' "$counts" "$warnings")

existing=$(gh issue list --label "$label" --state open --json number --jq '.[0].number // empty' 2>/dev/null || true)
if [ -n "$existing" ]; then
  gh issue comment "$existing" --body "$title"$'\n\n'"$body"
else
  gh issue create --title "$title" --body "$body" --label "$label"
fi
