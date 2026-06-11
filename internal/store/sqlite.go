package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"github.com/dong4j/starcat-wiki-api/internal/probe"
)

func getEnvInt(key string, def int) int {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			return v
		}
	}
	return def
}

// SQLiteStore SQLite 存储实现。
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 创建并初始化 SQLite 存储。
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	log.Printf("[store] sqlite opened at %s", dsn)
	return &SQLiteStore{db: db}, nil
}

// GetProbes 获取某个 repo 的全部探测结果（不包含 probing 状态）。
func (s *SQLiteStore) GetProbes(ctx context.Context, owner, repo string) ([]probe.ProbeResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT source, status, url, probe_method, http_status, matched_signals, expires_at
		FROM doc_probes WHERE owner = ? AND repo = ?
	`, owner, repo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []probe.ProbeResult
	for rows.Next() {
		var res probe.ProbeResult
		var signalsJSON, expiresAtStr string
		var httpStatus sql.NullInt32

		err := rows.Scan(&res.Source, &res.Status, &res.URL, &res.ProbeMethod, &httpStatus, &signalsJSON, &expiresAtStr)
		if err != nil {
			return nil, err
		}

		if httpStatus.Valid {
			val := int(httpStatus.Int32)
			res.HTTPStatus = &val
		}

		if signalsJSON != "" {
			_ = json.Unmarshal([]byte(signalsJSON), &res.MatchedSignals)
		}

		results = append(results, res)
	}
	return results, nil
}

// UpsertProbe 写入/更新单条探测结果（v2：含 attempt/last_error/next_retry_at 重置）。
func (s *SQLiteStore) UpsertProbe(ctx context.Context, owner, repo string, result probe.ProbeResult) error {
	checkedAt := time.Now().Format(time.RFC3339)
	ttl := cacheTTL(result.Status)
	expiresAt := time.Now().Add(ttl).Format(time.RFC3339)

	signals, _ := json.Marshal(result.MatchedSignals)
	var httpStatus interface{}
	if result.HTTPStatus != nil {
		httpStatus = *result.HTTPStatus
	}

	// 成功结果重置 attempt 和 last_error/next_retry_at
	resetAttempt := 0
	var resetLastError, resetNextRetry interface{}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO doc_probes
			(owner, repo, source, status, url, probe_method, http_status,
			 matched_signals, attempt, last_error, next_retry_at, checked_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner, repo, source) DO UPDATE SET
			status          = excluded.status,
			url             = excluded.url,
			probe_method    = excluded.probe_method,
			http_status     = excluded.http_status,
			matched_signals = excluded.matched_signals,
			attempt         = excluded.attempt,
			last_error      = excluded.last_error,
			next_retry_at   = excluded.next_retry_at,
			checked_at      = excluded.checked_at,
			expires_at      = excluded.expires_at
	`, owner, repo, result.Source, result.Status, result.URL, result.ProbeMethod,
		httpStatus, string(signals), resetAttempt, resetLastError, resetNextRetry,
		checkedAt, expiresAt)

	return err
}

// cacheTTL 返回各状态的缓存 TTL。
func cacheTTL(status probe.Status) time.Duration {
	switch status {
	case probe.StatusIndexed:
		return time.Duration(getEnvInt("CACHE_INDEXED_TTL_HOURS", 168)) * time.Hour
	case probe.StatusNotIndexed:
		return time.Duration(getEnvInt("CACHE_NOT_INDEXED_TTL_HOURS", 24)) * time.Hour
	case probe.StatusProbing:
		return time.Duration(getEnvInt("CACHE_PROBING_TTL_MINUTES", 10)) * time.Minute
	case probe.StatusError:
		return time.Duration(getEnvInt("CACHE_ERROR_TTL_MINUTES", 30)) * time.Minute
	default:
		return 6 * time.Hour
	}
}

// GetExpiredProbes 获取已过期的探测记录。
func (s *SQLiteStore) GetExpiredProbes(ctx context.Context, limit int) ([]ProbeRecord, error) {
	now := time.Now().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner, repo, source FROM doc_probes
		WHERE expires_at < ? AND status != 'probing'
		LIMIT ?
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ProbeRecord
	for rows.Next() {
		var r ProbeRecord
		if err := rows.Scan(&r.Owner, &r.Repo, &r.Source); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

// --- v2 新增方法 ---

// InsertPendingProbes 预插入 3 条 probing 记录（每个 source 一条）。
// INSERT OR IGNORE：已存在的记录不会被覆盖。
func (s *SQLiteStore) InsertPendingProbes(ctx context.Context, owner, repo string) error {
	now := time.Now()
	checkedAt := now.Format(time.RFC3339)
	probingTTL := time.Duration(getEnvInt("CACHE_PROBING_TTL_MINUTES", 10)) * time.Minute
	expiresAt := now.Add(probingTTL).Format(time.RFC3339)

	sources := []probe.Source{probe.SourceZread, probe.SourceDeepWiki, probe.SourceCodeWiki}
	for _, src := range sources {
		pageURL := probeURL(owner, repo, src)
		_, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO doc_probes
				(owner, repo, source, status, url, checked_at, expires_at, attempt)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		`, owner, repo, src, probe.StatusProbing, pageURL, checkedAt, expiresAt)
		if err != nil {
			return err
		}
	}
	return nil
}

// probeURL 根据 source 生成展示页 URL。
func probeURL(owner, repo string, src probe.Source) string {
	switch src {
	case probe.SourceZread:
		return "https://zread.ai/" + owner + "/" + repo
	case probe.SourceDeepWiki:
		return "https://deepwiki.com/" + owner + "/" + repo
	case probe.SourceCodeWiki:
		return "https://codewiki.google/github.com/" + owner + "/" + repo
	default:
		return ""
	}
}

// GetPendingProbes 获取所有 probing 状态的记录（宕机恢复用）。
func (s *SQLiteStore) GetPendingProbes(ctx context.Context) ([]ProbeRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT owner, repo, source FROM doc_probes WHERE status = 'probing'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ProbeRecord
	for rows.Next() {
		var r ProbeRecord
		if err := rows.Scan(&r.Owner, &r.Repo, &r.Source); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}
	// GetRetryableErrors 获取待重试的 error 记录（含 attempt）。
	func (s *SQLiteStore) GetRetryableErrors(ctx context.Context, maxAttempts int) ([]ProbeRecordWithAttempt, error) {
		now := time.Now().Format(time.RFC3339)
		rows, err := s.db.QueryContext(ctx, `
			SELECT owner, repo, source, attempt FROM doc_probes
			WHERE status = 'error'
			  AND attempt < ?
			  AND (next_retry_at IS NULL OR next_retry_at <= ?)
		`, maxAttempts, now)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var records []ProbeRecordWithAttempt
		for rows.Next() {
			var r ProbeRecordWithAttempt
			if err := rows.Scan(&r.Owner, &r.Repo, &r.Source, &r.Attempt); err != nil {
				return nil, err
			}
			records = append(records, r)
		}
		return records, nil
	}

	// IncrementAndResetProbing 原子递增 attempt 并将 error 重置为 probing。
	func (s *SQLiteStore) IncrementAndResetProbing(ctx context.Context, owner, repo string,
		source probe.Source, nextRetryAt string) error {

		_, err := s.db.ExecContext(ctx, `
			UPDATE doc_probes SET
				status = 'probing',
				attempt = attempt + 1,
				next_retry_at = ?
			WHERE owner = ? AND repo = ? AND source = ?
		`, nextRetryAt, owner, repo, source)

		return err
	}

	// AbandonMaxAttempts 将 attempt >= maxAttempts 的 error 记录标记为 not_indexed。
	func (s *SQLiteStore) AbandonMaxAttempts(ctx context.Context, maxAttempts int) (int, error) {
		checkedAt := time.Now().Format(time.RFC3339)
		ttl := cacheTTL(probe.StatusNotIndexed)
		expiresAt := time.Now().Add(ttl).Format(time.RFC3339)

		result, err := s.db.ExecContext(ctx, `
			UPDATE doc_probes SET
				status = 'not_indexed',
				checked_at = ?,
				expires_at = ?
			WHERE status = 'error' AND attempt >= ?
		`, checkedAt, expiresAt, maxAttempts)
		if err != nil {
			return 0, err
		}

		n, _ := result.RowsAffected()
		return int(n), nil
	}


// UpdateProbeResult 更新探测结果（probing/error → final）。
func (s *SQLiteStore) UpdateProbeResult(ctx context.Context, owner, repo string,
	source probe.Source, result probe.ProbeResult, attempt int, lastError string,
	nextRetryAt string) error {

	checkedAt := time.Now().Format(time.RFC3339)
	ttl := cacheTTL(result.Status)
	expiresAt := time.Now().Add(ttl).Format(time.RFC3339)

	signals, _ := json.Marshal(result.MatchedSignals)
	var httpStatus interface{}
	if result.HTTPStatus != nil {
		httpStatus = *result.HTTPStatus
	}

	var lastErrorVal, nextRetryVal interface{}
	if lastError != "" {
		lastErrorVal = lastError
	}
	if nextRetryAt != "" {
		nextRetryVal = nextRetryAt
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE doc_probes SET
			status          = ?,
			url             = ?,
			probe_method    = ?,
			http_status     = ?,
			matched_signals = ?,
			attempt         = ?,
			last_error      = ?,
			next_retry_at   = ?,
			checked_at      = ?,
			expires_at      = ?
		WHERE owner = ? AND repo = ? AND source = ?
	`, result.Status, result.URL, result.ProbeMethod,
		httpStatus, string(signals),
		attempt, lastErrorVal, nextRetryVal,
		checkedAt, expiresAt,
		owner, repo, source)

	return err
}

// MarkNotIndexed 强制标记为 not_indexed（反爬/放弃重试）。
func (s *SQLiteStore) MarkNotIndexed(ctx context.Context, owner, repo string,
	source probe.Source) error {

	checkedAt := time.Now().Format(time.RFC3339)
	ttl := cacheTTL(probe.StatusNotIndexed)
	expiresAt := time.Now().Add(ttl).Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx, `
		UPDATE doc_probes SET
			status = 'not_indexed',
			checked_at = ?,
			expires_at = ?
		WHERE owner = ? AND repo = ? AND source = ?
	`, checkedAt, expiresAt, owner, repo, source)

	return err
}

// ResetToProbing 将 error 记录重置为 probing 状态（定时重试入队）。
func (s *SQLiteStore) ResetToProbing(ctx context.Context, owner, repo string,
	source probe.Source, nextRetryAt string) error {

	_, err := s.db.ExecContext(ctx, `
		UPDATE doc_probes SET
			status = 'probing',
			next_retry_at = ?
		WHERE owner = ? AND repo = ? AND source = ?
	`, nextRetryAt, owner, repo, source)

	return err
}

// Close 关闭数据库连接。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
