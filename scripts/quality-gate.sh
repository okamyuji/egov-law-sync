#!/usr/bin/env bash
# quality-gate: pre-commitとCIが同じ順で実行する品質ゲート。どれか1つでも失敗したらcommitしない
# 使い方: bash scripts/quality-gate.sh  （make quality でも同じ）
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "$1 is required: $2" >&2; exit 1; }
}
need staticcheck "go install honnef.co/go/tools/cmd/staticcheck@latest"
need golangci-lint "brew install golangci-lint"
need govulncheck "go install golang.org/x/vuln/cmd/govulncheck@latest"

echo "==> gofmt"
test -z "$(gofmt -l .)" || { gofmt -l .; echo "gofmt: 上のファイルを整形してください" >&2; exit 1; }
echo "==> go vet"
go vet ./...
echo "==> layers"
bash tools/layers.sh
echo "==> staticcheck"
staticcheck ./...
echo "==> golangci-lint"
golangci-lint run ./...
echo "==> govulncheck"
govulncheck ./...
echo "==> go build"
go build ./...
echo "==> shelltest"
bash tools/next-version_test.sh
echo "==> doclint"
bash tools/doclint.sh
echo "==> go test (count=1 shuffle race cover) + crap"
make crap
echo "==> mutate"
make mutate
echo "==> e2e"
make e2e
echo "==> gitleaks"
if command -v gitleaks >/dev/null 2>&1; then
  gitleaks detect --no-git --source . --redact --no-banner
else
  echo "  (gitleaks not installed; CI runs the secret scan)"
fi
echo "all quality checks passed"
