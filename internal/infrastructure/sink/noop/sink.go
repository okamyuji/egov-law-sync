// Package noop ChunkSinkのnoop実装。vectorDBを持たない環境での動作確認用
package noop

import (
	"context"
	"log"

	"github.com/okamyuji/egov-law-sync/internal/application"
	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

var _ application.ChunkSink = (*Sink)(nil)

// Sink 受け取った件数を数えるだけのChunkSink
type Sink struct {
	Total int
}

// Put 件数を足して1行出す
func (s *Sink) Put(_ context.Context, chunks []law.Chunk) error {
	s.Total += len(chunks)
	log.Printf("noop sink: %d chunks (total %d)", len(chunks), s.Total)
	return nil
}
