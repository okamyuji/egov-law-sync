#!/usr/bin/env bash
# doclint: docs/ の設計文書と Go のコメント行に対する機械検査。docs/_quality/QUALITY_RUBRIC.md の規則本体。
set -u
root="$(cd "$(dirname "$0")/.." && pwd)"
target="${1:-$root}"
out="$(mktemp)"
trap 'rm -f "$out"' EXIT
# 全角文字と半角英数の間の半角スペース。コードブロックの中は対象外。表の中も対象。
# BSD awk は UTF-8 の文字クラスをバイト単位で照合して誤検知するため perl を使う
space_check() { # file mode(md|go)
  perl -CSD -Mutf8 -ne '
    BEGIN{$c=0; $mode=shift}
    if($mode eq "md"){ if(/^```/){$c=!$c;next} next if $c; s/`[^`]*`//g; }
    else { next unless m{^\s*//}; s{^\s*//\s*}{}; s{^[A-Za-z_][A-Za-z0-9_]*\s+(?!は)}{}; }
    print "[High] $ARGV:$. 全角と半角英数の間に半角スペースがあります\n"
      if /[\p{Han}\p{Hiragana}\p{Katakana}、。（）]\s+[A-Za-z0-9]/ || /[A-Za-z0-9%]\s+[\p{Han}\p{Hiragana}\p{Katakana}、。（）]/;
  ' "$2" "$1"
}
md_files=$(find "$target" -name '*.md' -not -path '*/_quality/*' -not -path '*/superpowers/*' -not -path '*/.git/*')
for f in $md_files; do
  grep -nE 'TBD|TODO|未定|後で決める' "$f" | sed "s|^\([0-9]*\):.*|[Critical] $f:\1 未決定の印があります|" >> "$out"
  space_check "$f" md >> "$out"
  awk 'BEGIN{code=0} /^```/{code=!code; next} code==0 && /\*\*[^*]+\*\*/ {print "[High] "FILENAME":"NR" アスタリスク2つの強調があります"}' "$f" >> "$out"
  if [ "$(basename "$f")" = "design.md" ]; then
    for sec in '## 目的' '## 決定表' '## 構成' '## 実装規約' '## データ' '## 処理の流れ' '## 異常判定' '## 通知' '## テスト' '## 未確定'; do
      grep -q "^$sec" "$f" || echo "[Medium] $f:0 必須節がありません: $sec" >> "$out"
    done
  fi
done
go_files=$(find "$target" -name '*.go' -not -path '*/.git/*')
for f in $go_files; do
  space_check "$f" go >> "$out"
  # ドキュメントコメントは「名前 説明」。「名前 は 説明」の形は不可
  perl -CSD -Mutf8 -ne 'print "[High] $ARGV:$. ドキュメントコメントに「名前 は」の形があります\n" if m{^\s*//\s*[A-Za-z_][A-Za-z0-9_]*\s+は}' "$f" >> "$out"
done
cat "$out"
crit=$(grep -c '^\[Critical\]' "$out"); high=$(grep -c '^\[High\]' "$out"); med=$(grep -c '^\[Medium\]' "$out"); low=$(grep -c '^\[Low\]' "$out")
echo "Critical $crit / High $high / Medium $med / Low $low"
[ "$crit" -eq 0 ] && [ "$high" -eq 0 ]
