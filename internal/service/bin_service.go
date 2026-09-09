package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/beon/bin-api/internal/model"
	"github.com/beon/bin-api/internal/repository"
	"github.com/beon/bin-api/internal/requestcontext"
	"github.com/beon/bin-api/pkg/binlist"
	"github.com/beon/bin-api/pkg/cache"
	"github.com/beon/bin-api/pkg/handyapi"
	"github.com/beon/bin-api/pkg/validator"
	"golang.org/x/sync/singleflight"
)

var (
	ErrEnrichmentUnavailable = errors.New("BIN enrichment unavailable")
	ErrEnrichmentRateLimited = errors.New("BIN enrichment providers rate limited")
)

type handyLookupClient interface {
	Lookup(context.Context, string) (*handyapi.Response, error)
}

type binlistLookupClient interface {
	Lookup(context.Context, string) (*binlist.Response, error)
}

// EnrichmentConfig controls the private fallback chain.
type EnrichmentConfig struct {
	Enabled           bool
	HandyAPIKey       string
	HandyMonthlyLimit int64
}

// BINService handles BIN lookup business logic
type BINService struct {
	repo              repository.BINRepository
	cache             *cache.Cache
	misses            *cache.Cache
	handy             handyLookupClient
	binlist           binlistLookupClient
	handyMonthlyLimit int64
	enrich            bool
	group             singleflight.Group
}

// New creates a BINService
func New(repo repository.BINRepository, config EnrichmentConfig) *BINService {
	limit := config.HandyMonthlyLimit
	if limit <= 0 {
		limit = 3_000
	}
	service := &BINService{
		repo:              repo,
		cache:             cache.New(30*time.Minute, 20_000),
		misses:            cache.New(5*time.Minute, 50_000),
		binlist:           binlist.New(),
		handyMonthlyLimit: limit,
		enrich:            config.Enabled,
	}
	if key := strings.TrimSpace(config.HandyAPIKey); key != "" {
		service.handy = handyapi.New(key)
	}
	return service
}

// Lookup returns BIN info. Priority: cache -> PostgreSQL -> HandyAPI -> binlist.net.
func (s *BINService) Lookup(ctx context.Context, bin string) (*model.BINResponse, error) {
	if v, ok := s.cache.Get(bin); ok {
		if resp, ok := v.(*model.BINResponse); ok {
			return resp, nil
		}
	}
	if _, ok := s.misses.Get(bin); ok {
		return nil, nil
	}

	value, err, _ := s.group.Do(bin, func() (interface{}, error) {
		record, err := s.repo.GetByBIN(ctx, bin)
		if err != nil {
			return nil, fmt.Errorf("db lookup: %w", err)
		}
		if record != nil {
			resp := record.ToResponse()
			s.cache.Set(bin, &resp)
			return &resp, nil
		}

		if !s.enrich {
			s.misses.Set(bin, true)
			return nil, nil
		}

		newRecord, err := s.lookupEnrichment(ctx, bin)
		if err != nil {
			if errors.Is(err, ErrEnrichmentRateLimited) {
				return nil, err
			}
			return nil, fmt.Errorf("%w: %v", ErrEnrichmentUnavailable, err)
		}
		if newRecord == nil {
			s.misses.Set(bin, true)
			return nil, nil
		}
		if err := s.repo.Upsert(ctx, newRecord); err != nil {
			return nil, fmt.Errorf("%w: persist %s enrichment: %v", ErrEnrichmentUnavailable, newRecord.Source, err)
		}

		resp := newRecord.ToResponse()
		s.cache.Set(bin, &resp)
		return &resp, nil
	})
	if err != nil || value == nil {
		return nil, err
	}
	resp, ok := value.(*model.BINResponse)
	if !ok {
		return nil, errors.New("unexpected lookup result type")
	}
	return resp, nil
}

func (s *BINService) lookupEnrichment(ctx context.Context, bin string) (*model.BIN, error) {
	var providerErrors []error
	if s.handy != nil {
		periodStart := startOfUTCMonth(time.Now())
		allowed, err := s.repo.TryConsumeEnrichmentQuota(
			ctx, "handy_api", periodStart, s.handyMonthlyLimit,
		)
		if err != nil {
			providerErrors = append(providerErrors, err)
			slog.WarnContext(ctx, "enrichment quota reservation failed",
				"request_id", requestcontext.RequestID(ctx),
				"provider", "handy_api", "bin", bin, "error", err)
		} else if allowed {
			response, lookupErr := s.handy.Lookup(ctx, bin)
			if lookupErr != nil {
				providerErrors = append(providerErrors, fmt.Errorf("HandyAPI: %w", lookupErr))
				slog.WarnContext(ctx, "enrichment provider failed",
					"request_id", requestcontext.RequestID(ctx),
					"provider", "handy_api", "bin", bin, "error", lookupErr)
			} else if response != nil {
				record := fromHandyAPI(bin, response)
				if validEnrichmentRecord(record) {
					return record, nil
				}
				providerErrors = append(providerErrors, errors.New("HandyAPI returned an invalid response"))
				slog.WarnContext(ctx, "enrichment provider returned invalid data",
					"request_id", requestcontext.RequestID(ctx),
					"provider", "handy_api", "bin", bin)
			}
		} else {
			providerErrors = append(providerErrors, errors.New("HandyAPI monthly quota exhausted"))
		}
	}

	response, err := s.binlist.Lookup(ctx, bin)
	if err != nil {
		providerErrors = append(providerErrors, fmt.Errorf("binlist.net: %w", err))
		slog.WarnContext(ctx, "enrichment provider failed",
			"request_id", requestcontext.RequestID(ctx),
			"provider", "binlist_net", "bin", bin, "error", err)
		if errors.Is(err, binlist.ErrRateLimited) {
			return nil, fmt.Errorf("%w: %v", ErrEnrichmentRateLimited, errors.Join(providerErrors...))
		}
		return nil, errors.Join(providerErrors...)
	}
	if response == nil {
		return nil, nil
	}
	record := fromBinlist(bin, response)
	if !validEnrichmentRecord(record) {
		providerErrors = append(providerErrors, errors.New("binlist.net returned an invalid response"))
		return nil, errors.Join(providerErrors...)
	}
	return record, nil
}

func startOfUTCMonth(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// Stats returns total BIN count
func (s *BINService) Stats(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}

// Close stops service-owned background workers.
func (s *BINService) Close() {
	s.cache.Close()
	s.misses.Close()
}

func validRemoteResponse(r *binlist.Response) bool {
	if r == nil {
		return false
	}
	return validEnrichmentRecord(fromBinlist("411111", r))
}

func validEnrichmentRecord(record *model.BIN) bool {
	if record == nil || !validator.IsValidBIN(record.BIN) {
		return false
	}
	fields := []struct {
		value string
		max   int
	}{
		{record.Brand, 50},
		{record.Type, 20},
		{record.Category, 50},
		{record.BankName, 200},
		{record.BankURL, 200},
		{record.BankPhone, 100},
		{record.CountryName, 100},
		{record.CountryCurrency, 10},
	}
	for _, field := range fields {
		if len(strings.TrimSpace(field.value)) > field.max {
			return false
		}
	}
	if code := strings.TrimSpace(record.CountryCode); code != "" && !isASCIIAlpha(code, 2) {
		return false
	}
	if !validCoordinate(record.CountryLatitude, -90, 90) ||
		!validCoordinate(record.CountryLong, -180, 180) {
		return false
	}
	return strings.TrimSpace(record.Brand) != "" ||
		strings.TrimSpace(record.BankName) != "" ||
		strings.TrimSpace(record.CountryCode) != ""
}

func validCoordinate(value *float64, min, max float64) bool {
	return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= min && *value <= max)
}

func isASCIIAlpha(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for i := range len(value) {
		if (value[i] < 'A' || value[i] > 'Z') && (value[i] < 'a' || value[i] > 'z') {
			return false
		}
	}
	return true
}

func fromBinlist(bin string, r *binlist.Response) *model.BIN {
	return &model.BIN{
		BIN:             bin,
		Brand:           strings.ToLower(strings.TrimSpace(r.Scheme)),
		Type:            strings.ToLower(strings.TrimSpace(r.Type)),
		Category:        strings.ToLower(strings.TrimSpace(r.Brand)),
		BankName:        strings.TrimSpace(r.Bank.Name),
		BankURL:         strings.TrimSpace(r.Bank.URL),
		BankPhone:       strings.TrimSpace(r.Bank.Phone),
		CountryName:     strings.TrimSpace(r.Country.Name),
		CountryCode:     strings.ToUpper(strings.TrimSpace(r.Country.Alpha2)),
		CountryCurrency: strings.ToUpper(strings.TrimSpace(r.Country.Currency)),
		CountryLatitude: r.Country.Latitude,
		CountryLong:     r.Country.Longitude,
		Prepaid:         r.Prepaid,
		Source:          "binlist_net",
	}
}

func fromHandyAPI(bin string, r *handyapi.Response) *model.BIN {
	return &model.BIN{
		BIN:         bin,
		Brand:       strings.ToLower(strings.TrimSpace(r.Scheme)),
		Type:        strings.ToLower(strings.TrimSpace(r.Type)),
		Category:    strings.ToLower(strings.TrimSpace(r.CardTier)),
		BankName:    strings.TrimSpace(r.Issuer),
		CountryName: strings.TrimSpace(r.Country.Name),
		CountryCode: strings.ToUpper(strings.TrimSpace(r.Country.Alpha2)),
		Source:      "handy_api",
	}
}
