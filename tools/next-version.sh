#!/usr/bin/env bash
# next-version: VERSION（MAJOR.MINOR）と既存Releaseのタグから次のタグ v<MAJOR>.<MINOR>.<PATCH> を出す
# 使い方: tools/next-version.sh  （カレントのgitリポジトリで gh が使えること）
set -eu

version=$(tr -d '[:space:]' < VERSION)
if ! printf '%s' "$version" | grep -Eq '^[0-9]+\.[0-9]+$'; then
  echo "VERSION must be MAJOR.MINOR, got: $version" >&2
  exit 1
fi

# checkoutはタグを持たないので、Releaseの一覧から取る
tags=$(gh release list --limit 1000 --json tagName --jq '.[].tagName')
# 同じMAJOR.MINORのPATCH最大値。無ければ0。max+1は構成上、既存タグと重複しない
escaped=${version//./\\.}
max=$(printf '%s\n' "$tags" | sed -nE "s/^v$escaped\.([0-9]+)$/\1/p" | sort -n | tail -1)
next="v$version.$(( ${max:-0} + 1 ))"
printf '%s\n' "$next"
