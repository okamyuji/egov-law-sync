#!/usr/bin/env bash
# crap: gocyclo の出力と go tool cover -func の出力を「ファイル:行」で突き合わせ、CRAP値を計算する。
# 使い方: crap.sh <gocyclo出力> <cover -func出力> <上限>
set -u
cyclo="$1"; cover="$2"; max="$3"
# gocyclo: "<cc> <pkg> <func> <path>:<line>:<col>"、cover -func: "<path>:<line>:\t<func>\t<cov>%"
# 突き合わせの鍵はパス末尾の「ファイル名:行」。同名メソッドが同じファイルにあっても行が違うので衝突しない
awk -v max="$max" '
  function key(p) { n=split(p, seg, "/"); return seg[n] }
  NR==FNR { loc=$4; sub(/:[0-9]+$/, "", loc); cc[key(loc)]=$1; name[key(loc)]=$3; next }
  /^total:/ { next }
  { loc=$1; sub(/:$/, "", loc); k=key(loc); if (!(k in cc)) next;
    matched++; cov=$3; sub("%","",cov); c=cc[k]; v=cov/100; crap=c*c*(1-v)^3+c;
    if (crap > max) { printf "%s %s cc=%d cov=%.0f%% crap=%.1f\n", loc, name[k], c, cov, crap; bad++ } }
  END {
    if (matched == 0) { print "crap: no function matched between gocyclo and cover output"; exit 1 }
    if (bad) { print bad " function(s) exceed CRAP " max; exit 1 }
    print "crap: " matched " functions checked, all <= " max
  }
' "$cyclo" "$cover"
