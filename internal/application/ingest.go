package application

import (
	"context"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

// ingestBatch 1回のPutに渡す上限。vectorDBのAPIは数百件単位の一括登録が普通で、1件ずつでは遅い
const ingestBatch = 100

// IngestOptions ingestサブコマンドの入力
type IngestOptions struct {
	TextDir string
}

// Ingester text-dirのJSONLを読んでChunkSinkへ流す。正本には触れないのでruns/には書かない
type Ingester interface {
	Run(ctx context.Context, o IngestOptions) (Result, error)
}

type ingester struct {
	d Deps
}

// NewIngester Ingesterを作る
func NewIngester(d Deps) Ingester {
	return &ingester{d: d}
}

func (g *ingester) Run(ctx context.Context, o IngestOptions) (Result, error) {
	rec := newRecord("ingest", g.d.Clock.Now())
	batch := make([]law.Chunk, 0, ingestBatch)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := g.d.Sink.Put(ctx, batch); err != nil {
			return err
		}
		rec.Counts["chunks"] += len(batch)
		rec.Counts["batches"]++
		batch = batch[:0]
		return nil
	}
	err := g.d.Source.Each(o.TextDir, func(c law.Chunk) error {
		batch = append(batch, c)
		if len(batch) == ingestBatch {
			return flush()
		}
		return nil
	})
	if err == nil {
		err = flush()
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Record: rec}, nil
}
