package store

import (
	"database/sql"
	"log"
)

// createSchema 初始化 wiki-api 全部数据表与索引。
//
// 全新服务,无版本迁移:任何现存 *.db 直接 rm 即可。首启动调一次
// CREATE TABLE IF NOT EXISTS 即可,不做 destructive migration。
func createSchema(db *sql.DB) error {
	log.Println("[migrate] createSchema: doc_probes table")
	_, err := db.Exec(`
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
	return err
}
