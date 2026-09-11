package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// useCase Optionsを受けて実行できるユースケース
type useCase[T any] interface {
	Run(context.Context, T) (application.Result, error)
}

// execute 引数を解釈し、parse成功後にDepsを組み立ててユースケースを実行する。--helpや引数誤りでmanifestディレクトリを作らないための順序。parse失敗は2、実行エラーは1
func execute[T any, U useCase[T]](ctx context.Context, args []string, stdout, stderr io.Writer,
	parse func([]string) (T, error), newUseCase func(application.Deps) U) int {
	o, err := parse(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "%v\n%s", err, usage)
		return 2
	}
	deps, err := buildDeps()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	res, err := newUseCase(deps).Run(ctx, o)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return res.ExitCode
}

// newFlagSet 自前でusageを出すため、flagパッケージ自身の出力は捨てる
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// commonFlags 3つのサブコマンドに共通のフラグを登録する
func commonFlags(fs *flag.FlagSet) (releaseTag, xmlDir, zipPath *string) {
	releaseTag = fs.String("release-tag", "", "GitHub Releaseのタグ")
	xmlDir = fs.String("xml-dir", "bin/xml", "XMLの保存先ディレクトリ")
	zipPath = fs.String("zip-path", "bin/laws-xml.zip", "リリース用zipの出力先")
	return releaseTag, xmlDir, zipPath
}

func parseBootstrap(args []string) (application.BootstrapOptions, error) {
	fs := newFlagSet("bootstrap")
	releaseTag, xmlDir, zipPath := commonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return application.BootstrapOptions{}, err
	}
	return application.BootstrapOptions{ReleaseTag: *releaseTag, XMLDir: *xmlDir, ZipPath: *zipPath}, nil
}

func parseWeekly(args []string) (application.WeeklyOptions, error) {
	fs := newFlagSet("weekly")
	releaseTag, xmlDir, zipPath := commonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return application.WeeklyOptions{}, err
	}
	return application.WeeklyOptions{ReleaseTag: *releaseTag, XMLDir: *xmlDir, ZipPath: *zipPath}, nil
}

func parseDaily(args []string) (application.DailyOptions, error) {
	fs := newFlagSet("daily")
	releaseTag, xmlDir, zipPath := commonFlags(fs)
	from := fs.String("from", "", "対象範囲の開始日 YYYY-MM-DD")
	to := fs.String("to", "", "対象範囲の終了日 YYYY-MM-DD")
	force := fs.Bool("force", false, "異常判定を無視して適用する")
	if err := fs.Parse(args); err != nil {
		return application.DailyOptions{}, err
	}
	o := application.DailyOptions{Force: *force, ReleaseTag: *releaseTag, XMLDir: *xmlDir, ZipPath: *zipPath}
	var err error
	if o.From, err = parseOptionalDate(*from); err != nil {
		return application.DailyOptions{}, err
	}
	if o.To, err = parseOptionalDate(*to); err != nil {
		return application.DailyOptions{}, err
	}
	return o, nil
}

// parseOptionalDate 空文字は未指定として扱い、ゼロ値のlaw.Dateを返す
func parseOptionalDate(s string) (law.Date, error) {
	if s == "" {
		return "", nil
	}
	return law.ParseDate(s)
}
