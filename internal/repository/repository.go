package repository

import (
	"context"
	"time"

	"github.com/beon/bin-api/internal/model"
)

// BINRepository defines the contract for BIN data access
type BINRepository interface {
	GetByBIN(ctx context.Context, bin string) (*model.BIN, error)
	Upsert(ctx context.Context, b *model.BIN) error
	Count(ctx context.Context) (int64, error)
	TryConsumeEnrichmentQuota(ctx context.Context, provider string, periodStart time.Time, limit int64) (bool, error)
}
