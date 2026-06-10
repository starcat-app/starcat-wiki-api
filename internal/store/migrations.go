package store

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	log.Printf("[migrate] current schema version: %d", version)

	if version < 1 {
		if err := migrateV1(db); err != nil {
			return err
		}
	}
	return nil
}

func setUserVersion(tx *sql.Tx, version int) error {
	_, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version))
	return err
}

func migrateV1(db *sql.DB) error {
	log.Println("[migrate] running migrateV1: create doc_probes table")

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS doc_probes (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			owner           TEXT NOT NULL,
			repo            TEXT NOT NULL,
			source          TEXT NOT NULL CHECK(source IN ('deepwiki','codewiki','zread')),
			status          TEXT NOT NULL,
			url             TEXT NOT NULL,
			confidence      TEXT,
			probe_method    TEXT,
			http_status     INTEGER,
			matched_signals TEXT,
			checked_at      TEXT NOT NULL,
			expires_at      TEXT NOT NULL,
			UNIQUE(owner, repo, source)
		);
		CREATE INDEX IF NOT EXISTS idx_doc_probes_lookup  ON doc_probes(owner, repo, expires_at);
		CREATE INDEX IF NOT EXISTS idx_doc_probes_refresh ON doc_probes(expires_at);
	`)
	if err != nil {
		return err
	}

	if err := setUserVersion(tx, 1); err != nil {
		return err
	}

	return tx.Commit()
}
