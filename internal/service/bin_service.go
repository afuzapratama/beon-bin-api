package service

import (
	"fmt"
	"log"
	"time"

	"github.com/beon/bin-api/internal/model"
	"github.com/beon/bin-api/internal/repository"
	"github.com/beon/bin-api/pkg/binlist"
	"github.com/beon/bin-api/pkg/cache"
)

// BINService handles BIN lookup business logic
type BINService struct {
	repo   repository.BINRepository
	cache  *cache.Cache
	remote *binlist.Client
	enrich bool
}

// New creates a BINService
func New(repo repository.BINRepository, enrichEnabled bool) *BINService {
	return &BINService{
		repo:   repo,
		cache:  cache.New(30 * time.Minute),
		remote: binlist.New(),
		enrich: enrichEnabled,
	}
}

// Lookup returns BIN info. Priority: cache -> local DB -> remote API
func (s *BINService) Lookup(bin string) (*model.BINResponse, error) {
	if v, ok := s.cache.Get(bin); ok {
		if resp, ok := v.(*model.BINResponse); ok {
			return resp, nil
		}
	}

	record, err := s.repo.GetByBIN(bin)
	if err != nil {
		return nil, fmt.Errorf("db lookup: %w", err)
	}
	if record != nil {
		resp := record.ToResponse()
		s.cache.Set(bin, &resp)
		return &resp, nil
	}

	if !s.enrich {
		return nil, nil
	}

	remoteData, err := s.remote.Lookup(bin)
	if err != nil {
		log.Printf("warn: binlist.net enrichment failed for %s: %v", bin, err)
		return nil, nil
	}
	if remoteData == nil {
		return nil, nil
	}

	newRecord := fromRemote(bin, remoteData)
	if err := s.repo.Upsert(newRecord); err != nil {
		log.Printf("warn: failed to persist enriched BIN %s: %v", bin, err)
	}

	resp := newRecord.ToResponse()
	s.cache.Set(bin, &resp)
	return &resp, nil
}

// Stats returns total BIN count
func (s *BINService) Stats() (int64, error) {
	return s.repo.Count()
}

func fromRemote(bin string, r *binlist.Response) *model.BIN {
	return &model.BIN{
		BIN:             bin,
		Brand:           r.Scheme,
		Type:            r.Type,
		Category:        "",
		BankName:        r.Bank.Name,
		BankURL:         r.Bank.URL,
		BankPhone:       r.Bank.Phone,
		CountryName:     r.Country.Name,
		CountryCode:     r.Country.Alpha2,
		CountryCurrency: r.Country.Currency,
		CountryLatitude: r.Country.Latitude,
		CountryLong:     r.Country.Longitude,
		Prepaid:         r.Prepaid,
		Source:          "binlist_net",
	}
}
