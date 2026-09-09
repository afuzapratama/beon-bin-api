package binlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const baseURL = "https://lookup.binlist.net"

var (
	ErrRateLimited = errors.New("binlist.net local rate limit reached")
	ErrCircuitOpen = errors.New("binlist.net circuit breaker is open")
)

// Client is an HTTP client for the binlist.net public API
type Client struct {
	http    *http.Client
	baseURL string
	limiter *rate.Limiter

	mu                  sync.Mutex
	consecutiveFailures int
	openUntil           time.Time
}

// Response mirrors the binlist.net JSON response
type Response struct {
	Number struct {
		Length int  `json:"length"`
		Luhn   bool `json:"luhn"`
	} `json:"number"`
	Scheme  string `json:"scheme"`
	Type    string `json:"type"`
	Brand   string `json:"brand"`
	Prepaid *bool  `json:"prepaid"`
	Country struct {
		Numeric   string   `json:"numeric"`
		Alpha2    string   `json:"alpha2"`
		Name      string   `json:"name"`
		Emoji     string   `json:"emoji"`
		Currency  string   `json:"currency"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"country"`
	Bank struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Phone string `json:"phone"`
		City  string `json:"city"`
	} `json:"bank"`
}

// New creates a new binlist.net client
func New() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
		// binlist.net documents a limit of 5 requests/hour with burst 5.
		limiter: rate.NewLimiter(rate.Every(12*time.Minute), 5),
	}
}

// Lookup queries binlist.net for a given BIN (6 to 8 digits)
func (c *Client) Lookup(ctx context.Context, bin string) (*Response, error) {
	if err := c.allowRequest(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, bin)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Version", "3")
	req.Header.Set("User-Agent", "beon-bin-api/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("binlist.net request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.recordSuccess()
		return nil, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		c.recordFailure()
		return nil, ErrRateLimited
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		c.recordFailure()
		return nil, fmt.Errorf("binlist.net returned status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binlist.net returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		c.recordFailure()
		return nil, err
	}

	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("parse binlist.net response: %w", err)
	}
	c.recordSuccess()
	return &result, nil
}

func (c *Client) allowRequest() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Before(c.openUntil) {
		return ErrCircuitOpen
	}
	if !c.limiter.Allow() {
		return ErrRateLimited
	}
	return nil
}

func (c *Client) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consecutiveFailures++
	if c.consecutiveFailures >= 3 {
		c.openUntil = time.Now().Add(5 * time.Minute)
		c.consecutiveFailures = 0
	}
}

func (c *Client) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consecutiveFailures = 0
	c.openUntil = time.Time{}
}
