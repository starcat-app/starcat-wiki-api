package store

import (
	"context"
	"github.com/dong4j/starcat-wiki-api/internal/probe"
)

type Store interface {
	GetProbes(ctx context.Context, owner, repo string) ([]probe.ProbeResult, error)
	UpsertProbe(ctx context.Context, owner, repo string, result probe.ProbeResult) error
	GetExpiredProbes(ctx context.Context, limit int) ([]ProbeRecord, error)
	Close() error
}

type ProbeRecord struct {
	Owner string
	Repo  string
	Source probe.Source
}
