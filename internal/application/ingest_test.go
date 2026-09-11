package application

import (
	"context"
	"errors"
	"testing"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
)

func nChunks(n int) []law.Chunk {
	out := make([]law.Chunk, n)
	for i := range n {
		out[i] = law.Chunk{LawID: "A", RevisionID: "A_1", Text: string(rune('a' + i%26))}
	}
	return out
}

func TestINV11BatchesOf100InOrder(t *testing.T) {
	src := &fakeSource{chunks: nChunks(250)}
	sink := &fakeSink{}
	res, err := NewIngester(Deps{Source: src, Sink: sink, Clock: &fakeClock{}}).Run(context.Background(), IngestOptions{TextDir: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.batches) != 3 || len(sink.batches[0]) != 100 || len(sink.batches[1]) != 100 || len(sink.batches[2]) != 50 {
		t.Fatalf("batch sizes wrong: %d", len(sink.batches))
	}
	total := 0
	for _, b := range sink.batches {
		for _, c := range b {
			if c.Text != src.chunks[total].Text {
				t.Fatalf("order broken at %d", total)
			}
			total++
		}
	}
	if total != 250 || res.Record.Counts["chunks"] != 250 || res.Record.Counts["batches"] != 3 || res.ExitCode != 0 {
		t.Fatalf("res=%+v total=%d", res.Record.Counts, total)
	}
}

func TestINV11StopsAtBrokenLine(t *testing.T) {
	src := &fakeSource{chunks: nChunks(250), broken: 150}
	sink := &fakeSink{}
	_, err := NewIngester(Deps{Source: src, Sink: sink, Clock: &fakeClock{}}).Run(context.Background(), IngestOptions{TextDir: "t"})
	if err == nil {
		t.Fatal("expected error")
	}
	// 100件目までは渡され、101〜150は壊れた行で止まるので渡されない
	if len(sink.batches) != 1 || len(sink.batches[0]) != 100 {
		t.Fatalf("batches = %d", len(sink.batches))
	}
}

func TestIngestPutErrorStops(t *testing.T) {
	src := &fakeSource{chunks: nChunks(250)}
	sink := &fakeSink{err: errors.New("db down")}
	_, err := NewIngester(Deps{Source: src, Sink: sink, Clock: &fakeClock{}}).Run(context.Background(), IngestOptions{TextDir: "t"})
	if err == nil || len(sink.batches) != 1 {
		t.Fatalf("err=%v batches=%d", err, len(sink.batches))
	}
}
