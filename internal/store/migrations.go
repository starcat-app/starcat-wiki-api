package store

import (
	"database/sql"
	"log"
)

// createSchema 初始化 wiki-api 全部数据表。
//
// v2 变更：
//   - 新增 attempt 重试轮次（默认 0）
//   - 新增 last_error 最后一次错误详情
//   - 新增 next_retry_at 下次可重试时间
//   - 移除 confidence 字段（状态简化）
//   - 索引新增 idx_doc_probes_retry 用于定时重试查询
func createSchema(db *sql.DB) error {
	log.Println("[migrate] createSchema: doc_probes table (v2)")
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS doc_probes (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			owner           TEXT NOT NULL,
			repo            TEXT NOT NULL,
			source          TEXT NOT NULL CHECK(source IN ('deepwiki','codewiki','zread')),
			status          TEXT NOT NULL,
			url             TEXT NOT NULL,
			probe_method    TEXT,
			http_status     INTEGER,
			matched_signals TEXT,
			attempt         INTEGER NOT NULL DEFAULT 0,
			last_error      TEXT,
			next_retry_at   TEXT,
			checked_at      TEXT NOT NULL,
			expires_at      TEXT NOT NULL,
			UNIQUE(owner, repo, source)
		);

		CREATE INDEX IF NOT EXISTS idx_doc_probes_lookup   ON doc_probes(owner, repo, expires_at);
		CREATE INDEX IF NOT EXISTS idx_doc_probes_refresh  ON doc_probes(expires_at);
		CREATE INDEX IF NOT EXISTS idx_doc_probes_retry    ON doc_probes(status, next_retry_at);
	`)
	if err != nil {
		return err
	}

	// 兼容旧表：如果表已存在但没有新字段，尝试 ALTER TABLE 添加
	migrateV2(db)

	return nil
}

// migrateV2 兼容旧表迁移：添加 v2 新增字段（如果不存在）。
func migrateV2(db *sql.DB) {
	migrations := []string{
		"ALTER TABLE doc_probes ADD COLUMN attempt INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE doc_probes ADD COLUMN last_error TEXT",
		"ALTER TABLE doc_probes ADD COLUMN next_retry_at TEXT",
	}

	for _, m := range migrations {
		_, err := db.Exec(m)
		if err != nil {
			// SQLite 不支持 IF NOT EXISTS on ALTER TABLE，
			// 重复添加会报错 duplicate column name，忽略即可
			log.Printf("[migrate] v2 migration (ignorable): %v", err)
		}
	}
}
