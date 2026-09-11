package noop

import (
	"context"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func TestPutCounts(t *testing.T) {
	s := &Sink{}
	if err := s.Put(context.Background(), make([]law.Chunk, 3)); err != nil || s.Total != 3 {
		t.Fatalf("total=%d err=%v", s.Total, err)
	}
}
