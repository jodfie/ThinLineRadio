// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Detects PostgreSQL B-tree index corruption (SQLSTATE XX002) and rebuilds the
// affected index so call ingest can recover without manual DBA intervention.

package main

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	indexNameFromErrorRe = regexp.MustCompile(`index "([^"]+)"`)
	insertTableFromErrorRe = regexp.MustCompile(`INSERT INTO "([^"]+)"`)
)

type postgresIndexHealer struct {
	mu         sync.Mutex
	locks      map[string]*sync.Mutex
	lastHealed map[string]time.Time
}

var pgIndexHealer = &postgresIndexHealer{
	locks:      make(map[string]*sync.Mutex),
	lastHealed: make(map[string]time.Time),
}

func isPostgresIndexCorruption(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlstate xx002") ||
		strings.Contains(msg, "right sibling's left-link doesn't match") ||
		strings.Contains(msg, "left-link doesn't match") ||
		strings.Contains(msg, "index corrupted") ||
		strings.Contains(msg, "broken on all main strategies")
}

func parseCorruptedIndexName(errMsg string) string {
	if m := indexNameFromErrorRe.FindStringSubmatch(errMsg); len(m) > 1 {
		return m[1]
	}
	return ""
}

func parseInsertTableName(errMsg string) string {
	if m := insertTableFromErrorRe.FindStringSubmatch(errMsg); len(m) > 1 {
		return m[1]
	}
	return ""
}

func validSQLIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

// healPostgresIndexCorruption rebuilds a corrupted PostgreSQL index named in err.
// Returns true when a heal was performed and the caller should retry the query.
func healPostgresIndexCorruption(db *Database, err error) bool {
	if db == nil || db.Sql == nil || db.Config.DbType != DbTypePostgresql || !isPostgresIndexCorruption(err) {
		return false
	}

	errMsg := err.Error()
	indexName := parseCorruptedIndexName(errMsg)
	if indexName == "" {
		tableName := parseInsertTableName(errMsg)
		if !validSQLIdentifier(tableName) {
			return false
		}
		writeLogStdout(fmt.Sprintf("postgres index heal: corruption detected on table %q, reindexing all indexes", tableName))
		if healErr := pgIndexHealer.healTable(db, tableName); healErr != nil {
			writeLogStdout(fmt.Sprintf("postgres index heal: reindex table %q failed: %v", tableName, healErr))
			return false
		}
		return true
	}

	if !validSQLIdentifier(indexName) {
		return false
	}

	writeLogStdout(fmt.Sprintf("postgres index heal: corruption detected on index %q, rebuilding", indexName))
	if healErr := pgIndexHealer.healIndex(db, indexName); healErr != nil {
		writeLogStdout(fmt.Sprintf("postgres index heal: reindex %q failed: %v", indexName, healErr))
		return false
	}
	writeLogStdout(fmt.Sprintf("postgres index heal: index %q rebuilt successfully", indexName))
	return true
}

func (h *postgresIndexHealer) lockFor(name string) *sync.Mutex {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.locks[name] == nil {
		h.locks[name] = &sync.Mutex{}
	}
	return h.locks[name]
}

func (h *postgresIndexHealer) recentlyHealed(name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.lastHealed[name]
	return ok && time.Since(t) < 2*time.Minute
}

func (h *postgresIndexHealer) markHealed(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastHealed[name] = time.Now()
}

func (h *postgresIndexHealer) healIndex(db *Database, indexName string) error {
	lock := h.lockFor(indexName)
	lock.Lock()
	defer lock.Unlock()

	if h.recentlyHealed(indexName) {
		return nil
	}

	if err := reindexPostgresIndex(db, indexName); err == nil {
		h.markHealed(indexName)
		return nil
	} else {
		writeLogStdout(fmt.Sprintf("postgres index heal: concurrent reindex %q failed (%v), trying blocking reindex", indexName, err))
	}

	if err := blockingReindexPostgresIndex(db, indexName); err == nil {
		h.markHealed(indexName)
		return nil
	} else {
		writeLogStdout(fmt.Sprintf("postgres index heal: blocking reindex %q failed (%v), trying drop and recreate", indexName, err))
	}

	if err := recreatePostgresIndex(db, indexName); err != nil {
		return err
	}
	h.markHealed(indexName)
	return nil
}

func (h *postgresIndexHealer) healTable(db *Database, tableName string) error {
	lock := h.lockFor("table:" + tableName)
	lock.Lock()
	defer lock.Unlock()

	if h.recentlyHealed("table:" + tableName) {
		return nil
	}

	query := fmt.Sprintf(`REINDEX TABLE CONCURRENTLY "%s"`, tableName)
	if _, err := db.Sql.Exec(query); err != nil {
		writeLogStdout(fmt.Sprintf("postgres index heal: concurrent reindex table %q failed (%v), trying blocking reindex", tableName, err))
		query = fmt.Sprintf(`REINDEX TABLE "%s"`, tableName)
		if _, err2 := db.Sql.Exec(query); err2 != nil {
			return err2
		}
	}
	h.markHealed("table:" + tableName)
	return nil
}

func reindexPostgresIndex(db *Database, indexName string) error {
	query := fmt.Sprintf(`REINDEX INDEX CONCURRENTLY "%s"`, indexName)
	_, err := db.Sql.Exec(query)
	return err
}

func blockingReindexPostgresIndex(db *Database, indexName string) error {
	query := fmt.Sprintf(`REINDEX INDEX "%s"`, indexName)
	_, err := db.Sql.Exec(query)
	return err
}

func recreatePostgresIndex(db *Database, indexName string) error {
	var indexDef string
	err := db.Sql.QueryRow(`
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND indexname = $1
	`, indexName).Scan(&indexDef)
	if err == sql.ErrNoRows {
		return fmt.Errorf("index %q not found in catalog", indexName)
	}
	if err != nil {
		return err
	}

	dropQuery := fmt.Sprintf(`DROP INDEX CONCURRENTLY IF EXISTS "%s"`, indexName)
	if _, err := db.Sql.Exec(dropQuery); err != nil {
		dropQuery = fmt.Sprintf(`DROP INDEX IF EXISTS "%s"`, indexName)
		if _, err = db.Sql.Exec(dropQuery); err != nil {
			return err
		}
	}

	createQuery := indexDef
	if !strings.Contains(createQuery, "CONCURRENTLY") {
		createQuery = strings.Replace(createQuery, "CREATE INDEX", "CREATE INDEX CONCURRENTLY", 1)
		createQuery = strings.Replace(createQuery, "CREATE UNIQUE INDEX", "CREATE UNIQUE INDEX CONCURRENTLY", 1)
	}
	if _, err := db.Sql.Exec(createQuery); err != nil {
		// Fall back to blocking create using the original definition.
		if _, err2 := db.Sql.Exec(indexDef); err2 != nil {
			return fmt.Errorf("concurrent recreate failed: %v; blocking recreate failed: %v", err, err2)
		}
	}
	return nil
}

// withPostgresIndexHeal runs fn and retries once after rebuilding a corrupted index.
func withPostgresIndexHeal(db *Database, fn func() error) error {
	err := fn()
	if err == nil || db == nil || db.Config.DbType != DbTypePostgresql {
		return err
	}
	if !healPostgresIndexCorruption(db, err) {
		return err
	}
	return fn()
}
