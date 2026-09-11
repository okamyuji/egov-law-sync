#!/usr/bin/env bash
# layers: application と domain が infrastructure と cmd に依存していないことを確認する（INV-12）
set -eu
deps=$(go list -deps ./internal/application ./internal/domain/...)
bad=$(printf '%s\n' "$deps" | grep -E 'internal/infrastructure|/cmd/' || true)
if [ -n "$bad" ]; then
  echo "layer violation:" >&2
  echo "$bad" >&2
  exit 1
fi
echo "layers: ok"
