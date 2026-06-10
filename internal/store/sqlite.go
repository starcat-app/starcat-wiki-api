package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
	"github.com/dong4j/starcat-wiki-api/internal/probe"
)

type SQLiteStore struct {
	db *sql.DB
}

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

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	log.Printf("[store] sqlite opened at %s", dsn)
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) GetProbes(ctx context.Context, owner, repo string) ([]probe.ProbeResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT source, status, url, confidence, probe_method, http_status, matched_signals, expires_at
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

		err := rows.Scan(&res.Source, &res.Status, &res.URL, &res.Confidence, &res.ProbeMethod, &httpStatus, &signalsJSON, &expiresAtStr)
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

func (s *SQLiteStore) UpsertProbe(ctx context.Context, owner, repo string, result probe.ProbeResult) error {
	now := time.Now()
	var ttl time.Duration

	switch result.Status {
	case probe.StatusIndexed, probe.StatusProbablyIndexed:
		ttl = 7 * 24 * time.Hour
	case probe.StatusNotIndexed:
		ttl = 24 * time.Hour
	case probe.StatusUnknown:
		ttl = 6 * time.Hour
	default:
		ttl = 30 * time.Minute
	}

	expiresAt := now.Add(ttl).Format(time.RFC3339)
	checkedAt := now.Format(time.RFC3339)

	signals, _ := json.Marshal(result.MatchedSignals)
	var httpStatus interface{}
	if result.HTTPStatus != nil {
		httpStatus = *result.HTTPStatus
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO doc_probes (owner, repo, source, status, url, confidence, probe_method, http_status, matched_signals, checked_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner, repo, source) DO UPDATE SET
			status = excluded.status,
			url = excluded.url,
			confidence = excluded.confidence,
			probe_method = excluded.probe_method,
			http_status = excluded.http_status,
			matched_signals = excluded.matched_signals,
			checked_at = excluded.checked_at,
			expires_at = excluded.expires_at
	`, owner, repo, result.Source, result.Status, result.URL, result.Confidence, result.ProbeMethod, httpStatus, string(signals), checkedAt, expiresAt)

	return err
}

func (s *SQLiteStore) GetExpiredProbes(ctx context.Context, limit int) ([]ProbeRecord, error) {
	now := time.Now().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner, repo, source FROM doc_probes WHERE expires_at < ? LIMIT ?
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

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
