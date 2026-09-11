package law

import (
	"testing"
	"time"
)

func TestDateAddCrossesMonth(t *testing.T) {
	d, err := ParseDate("2026-08-31")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Add(1); got != "2026-09-01" {
		t.Fatalf("got %s", got)
	}
	if got := d.Add(-31); got != "2026-07-31" {
		t.Fatalf("got %s", got)
	}
}

func TestDateOfUsesJST(t *testing.T) {
	// 入力のUTC22:30はJSTの翌日07:30になる
	utc := time.Date(2026, 9, 10, 22, 30, 0, 0, time.UTC)
	if got := DateOf(utc); got != "2026-09-11" {
		t.Fatalf("got %s", got)
	}
}

func TestParseDateRejectsBadFormat(t *testing.T) {
	for _, s := range []string{"20260911", "2026/09/11", "", "2026-13-01"} {
		if _, err := ParseDate(s); err == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}

func TestDateBefore(t *testing.T) {
	if !Date("2026-09-10").Before("2026-09-11") || Date("2026-09-11").Before("2026-09-11") {
		t.Fatal("Before is wrong")
	}
}

func TestDateString(t *testing.T) {
	if Date("2026-09-11").String() != "2026-09-11" {
		t.Fatal("String is wrong")
	}
}

func TestParseDateAccepts(t *testing.T) {
	d, err := ParseDate("2026-09-11")
	if err != nil {
		t.Fatal(err)
	}
	if d != "2026-09-11" {
		t.Fatalf("got %s", d)
	}
}
