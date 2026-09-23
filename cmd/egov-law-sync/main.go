// Package main egov-law-syncのエントリポイント。引数解釈、Depsの組み立て、終了コードの決定を担う
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/sync"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/clock"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/csv"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/egov"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/lawxml"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/sink/noop"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/zip"
)

const usage = `Usage: egov-law-sync <bootstrap|daily|weekly|ingest> [flags]

Subcommands:
  bootstrap   初回構築。laws.csv、revisions.csv、xml_index.csvを作る
  daily       日次同期。前回適用済みの翌日からJST前日までを対象にする
  weekly      週次のzip照合。laws.csvとxml_index.csvは変えず、破損だけ直す
  ingest <text-dir>   JSONLを読んでChunkSinkへ登録する（既定はnoop）

Flags (bootstrap, daily, weekly共通):
  --law-id string       法令IDを厳密に指定（専用のEGOV_MANIFEST_DIRが必要）
  --law-title string    法令名または略称の完全一致で指定（--law-idとは併用不可）
  --release-tag string    GitHub Releaseのタグ。空なら本文取得をしない
  --xml-dir string        XMLの保存先ディレクトリ（既定値 bin/xml）
  --zip-path string       リリース用zipの出力先（既定値 bin/laws-xml.zip）
  --text-dir string       MarkdownとJSONLの保存先ディレクトリ（既定値 bin/text）
  --text-zip-path string  MarkdownとJSONLのzipの出力先（既定値 bin/laws-text.zip）

Flags (bootstrapとdaily):
  --force         異常判定を無視して適用する。bootstrapでは既存laws.csvからの減少判定だけが対象

Flags (dailyのみ):
  --from string   対象範囲の開始日 YYYY-MM-DD。空なら自動で決める
  --to string     対象範囲の終了日 YYYY-MM-DD。空ならJST前日
`

func main() {
	// 実行の記録はruns/のJSONが持つので、stderrには時刻を付けない
	log.SetFlags(0)
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run 引数を解釈し、Depsを組み立て、ユースケースを実行して終了コードを返す
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	ctx := context.Background()
	switch args[0] {
	case "bootstrap":
		return execute(ctx, args[1:], stdout, stderr, parseBootstrap, application.NewBootstrapper)
	case "daily":
		return execute(ctx, args[1:], stdout, stderr, parseDaily, application.NewDailySyncer)
	case "weekly":
		return execute(ctx, args[1:], stdout, stderr, parseWeekly, application.NewWeeklyChecker)
	case "ingest":
		return execute(ctx, args[1:], stdout, stderr, parseIngest, application.NewIngester)
	default:
		fmt.Fprintf(stderr, "unknown subcommand: %s\n%s", args[0], usage)
		return 2
	}
}

// env 環境変数が空なら既定値を使う
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// atoiEnv 環境変数が空か数値でなければ既定値を使う
func atoiEnv(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// buildDeps manifestの保存先を作ってからDepsを組み立てる
func buildDeps() (application.Deps, error) {
	manifestDir := env("EGOV_MANIFEST_DIR", "manifest")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return application.Deps{}, err
	}
	c := egov.New(
		env("EGOV_V2_BASE", "https://laws.e-gov.go.jp/api/2"),
		env("EGOV_V1_BASE", "https://elaws.e-gov.go.jp/api/1"),
		env("EGOV_BULK_BASE", "https://laws.e-gov.go.jp"),
	)
	return application.Deps{
		Catalog: c, Revisions: c, XML: c, Updates: c, Daily: c, Bulk: c,
		Repo:        csv.New(manifestDir),
		Bundler:     zip.Bundler{},
		Text:        lawxml.Renderer{},
		Source:      lawxml.JSONL{},
		Sink:        &noop.Sink{},
		Clock:       clock.System{},
		Threshold:   sync.DefaultThresholds(),
		Concurrency: atoiEnv("EGOV_CONCURRENCY", 1),
	}, nil
}
