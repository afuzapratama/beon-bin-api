package repository

import "github.com/beon/bin-api/internal/model"

// BINRepository defines the contract for BIN data access
type BINRepository interface {
	GetByBIN(bin string) (*model.BIN, error)
	Upsert(b *model.BIN) error
	BatchUpsert(bins []*model.BIN) error
	Count() (int64, error)
}
