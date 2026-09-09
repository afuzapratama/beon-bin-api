package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/beon/bin-api/internal/model"
	"github.com/beon/bin-api/pkg/binlist"
	"github.com/beon/bin-api/pkg/handyapi"
)

type fakeRepository struct {
	mu        sync.Mutex
	record    *model.BIN
	getCalls  int
	upserts   []*model.BIN
	upsertErr error
	quota     bool
	quotaErr  error
}

func (r *fakeRepository) GetByBIN(context.Context, string) (*model.BIN, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCalls++
	return r.record, nil
}

func (r *fakeRepository) Upsert(_ context.Context, record *model.BIN) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserts = append(r.upserts, record)
	return r.upsertErr
}

func (*fakeRepository) Count(context.Context) (int64, error) { return 0, nil }

func (r *fakeRepository) TryConsumeEnrichmentQuota(context.Context, string, time.Time, int64) (bool, error) {
	return r.quota, r.quotaErr
}

func (r *fakeRepository) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getCalls
}

func (r *fakeRepository) lastUpsert() *model.BIN {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.upserts) == 0 {
		return nil
	}
	return r.upserts[len(r.upserts)-1]
}

type fakeHandyClient struct {
	response *handyapi.Response
	err      error
	calls    int
}

func (c *fakeHandyClient) Lookup(context.Context, string) (*handyapi.Response, error) {
	c.calls++
	return c.response, c.err
}

type fakeBinlistClient struct {
	response *binlist.Response
	err      error
	calls    int
}

func (c *fakeBinlistClient) Lookup(context.Context, string) (*binlist.Response, error) {
	c.calls++
	return c.response, c.err
}

func TestLookupCachesDatabaseHit(t *testing.T) {
	repo := &fakeRepository{record: &model.BIN{BIN: "411111", Brand: "visa"}}
	svc := New(repo, EnrichmentConfig{})

	for range 2 {
		result, err := svc.Lookup(context.Background(), "411111")
		if err != nil {
			t.Fatalf("Lookup returned error: %v", err)
		}
		if result == nil || result.BIN != "411111" {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
	if got := repo.calls(); got != 1 {
		t.Fatalf("repository calls = %d, want 1", got)
	}
}

func TestLookupNegativeCachesDatabaseMiss(t *testing.T) {
	repo := &fakeRepository{}
	svc := New(repo, EnrichmentConfig{})

	for range 2 {
		result, err := svc.Lookup(context.Background(), "411111")
		if err != nil {
			t.Fatalf("Lookup returned error: %v", err)
		}
		if result != nil {
			t.Fatalf("result = %+v, want nil", result)
		}
	}
	if got := repo.calls(); got != 1 {
		t.Fatalf("repository calls = %d, want 1", got)
	}
}

func TestValidRemoteResponse(t *testing.T) {
	latitude := 1.25
	longitude := 103.8
	response := &binlist.Response{Scheme: " visa ", Brand: " Platinum "}
	response.Country.Alpha2 = "id"
	response.Country.Latitude = &latitude
	response.Country.Longitude = &longitude
	if !validRemoteResponse(response) {
		t.Fatal("valid response was rejected")
	}

	record := fromBinlist("41111199", response)
	if record.Brand != "visa" || record.Category != "platinum" || record.CountryCode != "ID" {
		t.Fatalf("response was not normalized: %+v", record)
	}
}

func TestLookupUsesHandyFirstAndPersistsResult(t *testing.T) {
	repo := &fakeRepository{quota: true}
	handyResponse := &handyapi.Response{Status: "SUCCESS", Scheme: " VISA ", Type: " DEBIT ", Issuer: " Test Bank ", CardTier: " GOLD "}
	handyResponse.Country.Alpha2 = "id"
	handyResponse.Country.Name = "Indonesia"
	handy := &fakeHandyClient{response: handyResponse}
	binlistClient := &fakeBinlistClient{}
	svc := New(repo, EnrichmentConfig{Enabled: true, HandyAPIKey: "configured"})
	svc.handy = handy
	svc.binlist = binlistClient

	result, err := svc.Lookup(context.Background(), "99999999")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if result == nil || result.Brand != "visa" || result.Bank.Name != "Test Bank" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if handy.calls != 1 || binlistClient.calls != 0 {
		t.Fatalf("provider calls: Handy=%d Binlist=%d", handy.calls, binlistClient.calls)
	}
	stored := repo.lastUpsert()
	if stored == nil || stored.Source != "handy_api" || stored.CountryCode != "ID" {
		t.Fatalf("unexpected persisted record: %+v", stored)
	}
}

func TestLookupFallsBackToBinlistWhenHandyFails(t *testing.T) {
	repo := &fakeRepository{quota: true}
	handy := &fakeHandyClient{err: handyapi.ErrRateLimited}
	binlistResponse := &binlist.Response{Scheme: "mastercard", Type: "credit", Brand: "platinum"}
	binlistResponse.Country.Alpha2 = "SG"
	binlistClient := &fakeBinlistClient{response: binlistResponse}
	svc := New(repo, EnrichmentConfig{Enabled: true, HandyAPIKey: "configured"})
	svc.handy = handy
	svc.binlist = binlistClient

	result, err := svc.Lookup(context.Background(), "99999998")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if result == nil || result.Brand != "mastercard" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if handy.calls != 1 || binlistClient.calls != 1 {
		t.Fatalf("provider calls: Handy=%d Binlist=%d", handy.calls, binlistClient.calls)
	}
	if stored := repo.lastUpsert(); stored == nil || stored.Source != "binlist_net" {
		t.Fatalf("unexpected persisted record: %+v", stored)
	}
}

func TestLookupSkipsHandyWhenMonthlyQuotaIsExhausted(t *testing.T) {
	repo := &fakeRepository{quota: false}
	handy := &fakeHandyClient{}
	binlistResponse := &binlist.Response{Scheme: "visa"}
	binlistClient := &fakeBinlistClient{response: binlistResponse}
	svc := New(repo, EnrichmentConfig{Enabled: true, HandyAPIKey: "configured"})
	svc.handy = handy
	svc.binlist = binlistClient

	if _, err := svc.Lookup(context.Background(), "99999997"); err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if handy.calls != 0 || binlistClient.calls != 1 {
		t.Fatalf("provider calls: Handy=%d Binlist=%d", handy.calls, binlistClient.calls)
	}
}

func TestLookupReturnsUnavailableWhenFallbacksFail(t *testing.T) {
	repo := &fakeRepository{quota: true}
	svc := New(repo, EnrichmentConfig{Enabled: true, HandyAPIKey: "configured"})
	svc.handy = &fakeHandyClient{err: errors.New("handy down")}
	svc.binlist = &fakeBinlistClient{err: errors.New("binlist down")}

	result, err := svc.Lookup(context.Background(), "99999996")
	if result != nil || !errors.Is(err, ErrEnrichmentUnavailable) {
		t.Fatalf("result=%+v error=%v, want unavailable", result, err)
	}
}

func TestLookupReturnsRateLimitedWhenFinalProviderIsLimited(t *testing.T) {
	repo := &fakeRepository{quota: false}
	svc := New(repo, EnrichmentConfig{Enabled: true, HandyAPIKey: "configured"})
	svc.handy = &fakeHandyClient{}
	svc.binlist = &fakeBinlistClient{err: binlist.ErrRateLimited}

	result, err := svc.Lookup(context.Background(), "99999993")
	if result != nil || !errors.Is(err, ErrEnrichmentRateLimited) {
		t.Fatalf("result=%+v error=%v, want rate limited", result, err)
	}
}

func TestLookupReturnsNotFoundWhenBothProvidersMiss(t *testing.T) {
	repo := &fakeRepository{quota: true}
	handy := &fakeHandyClient{}
	binlistClient := &fakeBinlistClient{}
	svc := New(repo, EnrichmentConfig{Enabled: true, HandyAPIKey: "configured"})
	svc.handy = handy
	svc.binlist = binlistClient

	result, err := svc.Lookup(context.Background(), "99999995")
	if err != nil || result != nil {
		t.Fatalf("result=%+v error=%v, want clean not-found", result, err)
	}
	if handy.calls != 1 || binlistClient.calls != 1 {
		t.Fatalf("provider calls: Handy=%d Binlist=%d", handy.calls, binlistClient.calls)
	}
}

func TestLookupReturnsUnavailableWhenPersistenceFails(t *testing.T) {
	repo := &fakeRepository{quota: true, upsertErr: errors.New("database unavailable")}
	handyResponse := &handyapi.Response{Status: "SUCCESS", Scheme: "VISA"}
	svc := New(repo, EnrichmentConfig{Enabled: true, HandyAPIKey: "configured"})
	svc.handy = &fakeHandyClient{response: handyResponse}
	svc.binlist = &fakeBinlistClient{}

	result, err := svc.Lookup(context.Background(), "99999994")
	if result != nil || !errors.Is(err, ErrEnrichmentUnavailable) {
		t.Fatalf("result=%+v error=%v, want unavailable", result, err)
	}
}

func TestInvalidRemoteResponse(t *testing.T) {
	tests := map[string]*binlist.Response{
		"empty":        {},
		"country code": func() *binlist.Response { r := &binlist.Response{Scheme: "visa"}; r.Country.Alpha2 = "IDN"; return r }(),
		"oversized":    {Scheme: strings.Repeat("v", 51)},
		"nan latitude": func() *binlist.Response {
			r := &binlist.Response{Scheme: "visa"}
			v := math.NaN()
			r.Country.Latitude = &v
			return r
		}(),
		"bad longitude": func() *binlist.Response {
			r := &binlist.Response{Scheme: "visa"}
			v := 181.0
			r.Country.Longitude = &v
			return r
		}(),
	}
	for name, response := range tests {
		t.Run(name, func(t *testing.T) {
			if validRemoteResponse(response) {
				t.Fatal("invalid response was accepted")
			}
		})
	}
}
