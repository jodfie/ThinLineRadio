package main

import "testing"

func TestIsPostgresIndexCorruption(t *testing.T) {
	corrupt := `ERROR: right sibling's left-link doesn't match: block 7 links to 393 instead of expected 13 in index "calls_system_talkgroup_timestamp_idx" (SQLSTATE XX002)`
	if !isPostgresIndexCorruption(fmtError(corrupt)) {
		t.Fatal("expected corruption detection")
	}
	if isPostgresIndexCorruption(fmtError("duplicate key value violates unique constraint")) {
		t.Fatal("expected false for unrelated error")
	}
}

func TestParseCorruptedIndexName(t *testing.T) {
	msg := `in index "calls_system_talkgroup_timestamp_idx" (SQLSTATE XX002)`
	got := parseCorruptedIndexName(msg)
	if got != "calls_system_talkgroup_timestamp_idx" {
		t.Fatalf("got %q", got)
	}
}

func TestParseInsertTableName(t *testing.T) {
	msg := `in INSERT INTO "calls" ("audio", "audioFilename")`
	got := parseInsertTableName(msg)
	if got != "calls" {
		t.Fatalf("got %q", got)
	}
}

func TestValidSQLIdentifier(t *testing.T) {
	if !validSQLIdentifier("calls_system_talkgroup_timestamp_idx") {
		t.Fatal("expected valid")
	}
	if validSQLIdentifier(`calls"; DROP TABLE calls; --`) {
		t.Fatal("expected invalid")
	}
}

func fmtError(msg string) error {
	return &wrappedErr{msg: msg}
}

type wrappedErr struct{ msg string }

func (e *wrappedErr) Error() string { return e.msg }
