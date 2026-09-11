#!/usr/bin/env bash
# next-version_test: gh をスタブにして tools/next-version.sh の算出を検査する（INV-1、INV-2、INV-3）
set -u
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"
fail=0

# gh のスタブ。GH_STUB_TAGS を1行1タグで返す
cat > "$tmp/bin/gh" <<'EOF'
#!/usr/bin/env bash
if [ "${GH_STUB_FAIL:-}" = "1" ]; then echo "gh: boom" >&2; exit 1; fi
printf '%s\n' "${GH_STUB_TAGS:-}" | sed '/^$/d'
EOF
chmod +x "$tmp/bin/gh"

run_case() { # name version tags expected_out expected_code
  local name="$1" version="$2" tags="$3" want_out="$4" want_code="$5"
  printf '%s\n' "$version" > "$tmp/VERSION"
  local out code
  out=$(cd "$tmp" && PATH="$tmp/bin:$PATH" GH_STUB_TAGS="$tags" bash "$here/next-version.sh" 2>/dev/null); code=$?
  if [ "$out" != "$want_out" ] || [ "$code" != "$want_code" ]; then
    echo "FAIL $name: out=$out code=$code (want $want_out / $want_code)"; fail=1
  else
    echo "ok   $name"
  fi
}

run_case "INV-2 max patch plus one" "0.0" $'v0.0.3\nv0.0.1\nv0.1.2' "v0.0.4" 0
run_case "INV-2 no tags" "0.0" "" "v0.0.1" 0
run_case "INV-2 other minor ignored" "0.1" $'v0.0.3\nv0.1.2' "v0.1.3" 0
run_case "INV-2 non-semver tags ignored" "0.0" $'bootstrap-20260911T075435Z\nv0.0.2' "v0.0.3" 0
run_case "INV-1 bad version three parts" "0.0.1" "" "" 1
run_case "INV-1 bad version letters" "a.b" "" "" 1
printf '0.0\n' > "$tmp/VERSION"
out=$(cd "$tmp" && PATH="$tmp/bin:$PATH" GH_STUB_FAIL=1 bash "$here/next-version.sh" 2>/dev/null); code=$?
if [ "$code" != "1" ]; then echo "FAIL gh failure must exit 1 (code=$code)"; fail=1; else echo "ok   gh failure"; fi

exit $fail
