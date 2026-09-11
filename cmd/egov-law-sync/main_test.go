package main

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	o, err := parseDaily([]string{"--from", "2026-09-01", "--to", "2026-09-02", "--force", "--release-tag", "sync-1"})
	if err != nil || o.From != "2026-09-01" || !o.Force || o.ReleaseTag != "sync-1" {
		t.Fatalf("o=%+v err=%v", o, err)
	}
	if _, err := parseDaily([]string{"--from", "20260901"}); err == nil {
		t.Fatal("bad date must fail")
	}
}

func TestParseBootstrapDefaults(t *testing.T) {
	o, err := parseBootstrap(nil)
	if err != nil || o.XMLDir != "bin/xml" || o.ZipPath != "bin/laws-xml.zip" || o.ReleaseTag != "" {
		t.Fatalf("o=%+v err=%v", o, err)
	}
}

func TestParseTextFlags(t *testing.T) {
	o, err := parseDaily([]string{"--text-dir", "out/text", "--text-zip-path", "out/t.zip"})
	if err != nil || o.TextDir != "out/text" || o.TextZipPath != "out/t.zip" {
		t.Fatalf("o=%+v err=%v", o, err)
	}
	b, err := parseBootstrap(nil)
	if err != nil || b.TextDir != "bin/text" || b.TextZipPath != "bin/laws-text.zip" {
		t.Fatalf("defaults %+v err=%v", b, err)
	}
}

func TestParseWeeklyFlags(t *testing.T) {
	o, err := parseWeekly([]string{"--release-tag", "sync-2", "--xml-dir", "x", "--zip-path", "z"})
	if err != nil || o.ReleaseTag != "sync-2" || o.XMLDir != "x" || o.ZipPath != "z" {
		t.Fatalf("o=%+v err=%v", o, err)
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run([]string{"nope"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 || stdout.Len() == 0 {
		t.Fatalf("code=%d stdout=%s", code, stdout.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run(nil, &stdout, &stderr); code != 2 || stderr.Len() == 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}
