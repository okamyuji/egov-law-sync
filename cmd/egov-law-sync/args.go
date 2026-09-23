package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/csv"
	"github.com/okamyuji/egov-law-sync/internal/infrastructure/egov"
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
	if selection, ok := any(o).(interface{ LawSelection() application.Selection }); ok {
		scope := selection.LawSelection()
		if scope.LawID != "" || scope.LawTitle != "" {
			client := deps.Catalog.(*egov.Client)
			if scope.LawTitle != "" {
				id, resolveErr := client.ResolveTitle(ctx, scope.LawTitle)
				if resolveErr != nil {
					fmt.Fprintln(stderr, resolveErr)
					if errors.Is(resolveErr, egov.ErrTitleNotUnique) {
						return 2
					}
					return 1
				}
				scope.LawID = string(id)
			}
			id := law.LawID(scope.LawID)
			if checkErr := deps.Repo.(*csv.Repo).CheckScope(id); checkErr != nil {
				fmt.Fprintln(stderr, checkErr)
				return 2
			}
			if _, bootstrap := any(o).(application.BootstrapOptions); bootstrap {
				laws, _, fetchErr := client.ListByID(ctx, "", id)
				if fetchErr != nil {
					fmt.Fprintln(stderr, fetchErr)
					return 1
				}
				if len(laws) != 1 {
					fmt.Fprintf(stderr, "law ID %s: expected one exact match, got %d\n", id, len(laws))
					return 2
				}
			}
			deps.Scope = id
			deps.Catalog = application.ScopedCatalog{Source: client, ID: id}
		} else if checkErr := deps.Repo.(*csv.Repo).CheckScope(""); checkErr != nil {
			fmt.Fprintln(stderr, checkErr)
			return 2
		}
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

func selectionFlags(fs *flag.FlagSet) (lawID, lawTitle *string) {
	return fs.String("law-id", "", "対象の法令ID"), fs.String("law-title", "", "正式名称または略称")
}

func parseSelection(fs *flag.FlagSet, id, title string) (application.Selection, error) {
	if id != "" && title != "" {
		return application.Selection{}, errors.New("--law-id and --law-title cannot be combined")
	}
	if id != "" && !law.ValidID(id) {
		return application.Selection{}, fmt.Errorf("invalid --law-id: %q", id)
	}
	if fs.NArg() != 0 {
		return application.Selection{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return application.Selection{LawID: id, LawTitle: title}, nil
}

// commonFlags 3つのサブコマンドに共通のフラグを登録する
func commonFlags(fs *flag.FlagSet) (releaseTag, xmlDir, zipPath, textDir, textZipPath *string) {
	releaseTag = fs.String("release-tag", "", "GitHub Releaseのタグ")
	xmlDir = fs.String("xml-dir", "bin/xml", "XMLの保存先ディレクトリ")
	zipPath = fs.String("zip-path", "bin/laws-xml.zip", "リリース用zipの出力先")
	textDir = fs.String("text-dir", "bin/text", "MarkdownとJSONLの保存先ディレクトリ。空なら変換しない")
	textZipPath = fs.String("text-zip-path", "bin/laws-text.zip", "MarkdownとJSONLのzipの出力先")
	return releaseTag, xmlDir, zipPath, textDir, textZipPath
}

func parseBootstrap(args []string) (application.BootstrapOptions, error) {
	fs := newFlagSet("bootstrap")
	lawID, lawTitle := selectionFlags(fs)
	releaseTag, xmlDir, zipPath, textDir, textZipPath := commonFlags(fs)
	force := fs.Bool("force", false, "既存laws.csvからの減少判定を無視して適用する")
	if err := fs.Parse(args); err != nil {
		return application.BootstrapOptions{}, err
	}
	selection, err := parseSelection(fs, *lawID, *lawTitle)
	if err != nil {
		return application.BootstrapOptions{}, err
	}
	return application.BootstrapOptions{
		Selection: selection, Force: *force, ReleaseTag: *releaseTag, XMLDir: *xmlDir, ZipPath: *zipPath,
		TextDir: *textDir, TextZipPath: *textZipPath,
	}, nil
}

func parseWeekly(args []string) (application.WeeklyOptions, error) {
	fs := newFlagSet("weekly")
	lawID, lawTitle := selectionFlags(fs)
	releaseTag, xmlDir, zipPath, textDir, textZipPath := commonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return application.WeeklyOptions{}, err
	}
	selection, err := parseSelection(fs, *lawID, *lawTitle)
	if err != nil {
		return application.WeeklyOptions{}, err
	}
	return application.WeeklyOptions{
		Selection: selection, ReleaseTag: *releaseTag, XMLDir: *xmlDir, ZipPath: *zipPath,
		TextDir: *textDir, TextZipPath: *textZipPath,
	}, nil
}

func parseDaily(args []string) (application.DailyOptions, error) {
	fs := newFlagSet("daily")
	lawID, lawTitle := selectionFlags(fs)
	releaseTag, xmlDir, zipPath, textDir, textZipPath := commonFlags(fs)
	from := fs.String("from", "", "対象範囲の開始日 YYYY-MM-DD")
	to := fs.String("to", "", "対象範囲の終了日 YYYY-MM-DD")
	force := fs.Bool("force", false, "異常判定を無視して適用する")
	if err := fs.Parse(args); err != nil {
		return application.DailyOptions{}, err
	}
	selection, err := parseSelection(fs, *lawID, *lawTitle)
	if err != nil {
		return application.DailyOptions{}, err
	}
	o := application.DailyOptions{
		Selection: selection, Force: *force, ReleaseTag: *releaseTag, XMLDir: *xmlDir, ZipPath: *zipPath,
		TextDir: *textDir, TextZipPath: *textZipPath,
	}
	if o.From, err = parseOptionalDate(*from); err != nil {
		return application.DailyOptions{}, err
	}
	if o.To, err = parseOptionalDate(*to); err != nil {
		return application.DailyOptions{}, err
	}
	return o, nil
}

func parseIngest(args []string) (application.IngestOptions, error) {
	fs := newFlagSet("ingest")
	if err := fs.Parse(args); err != nil {
		return application.IngestOptions{}, err
	}
	if fs.NArg() != 1 {
		return application.IngestOptions{}, errors.New("ingest needs exactly one argument: <text-dir>")
	}
	return application.IngestOptions{TextDir: fs.Arg(0)}, nil
}

// parseOptionalDate 空文字は未指定として扱い、ゼロ値のlaw.Dateを返す
func parseOptionalDate(s string) (law.Date, error) {
	if s == "" {
		return "", nil
	}
	return law.ParseDate(s)
}
