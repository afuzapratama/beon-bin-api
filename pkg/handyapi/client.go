package handyapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const baseURL = "https://data.handyapi.com/bin"

var (
	ErrRateLimited = errors.New("HandyAPI local or upstream rate limit reached")
	ErrCircuitOpen = errors.New("HandyAPI circuit breaker is open")
)

// Client queries HandyAPI without ever exposing the configured API key.
type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string
	limiter *rate.Limiter

	mu                  sync.Mutex
	consecutiveFailures int
	openUntil           time.Time
}

// Response mirrors the HandyAPI BIN lookup response.
type Response struct {
	Status   string  `json:"Status"`
	Scheme   string  `json:"Scheme"`
	Type     string  `json:"Type"`
	Issuer   string  `json:"Issuer"`
	CardTier string  `json:"CardTier"`
	Country  Country `json:"Country"`
	Luhn     *bool   `json:"Luhn"`
}

// Country accepts HandyAPI's two observed response shapes: an object when
// country metadata exists, or an empty array when it does not.
type Country struct {
	Alpha2    string `json:"A2"`
	Alpha3    string `json:"A3"`
	Numeric   string `json:"N3"`
	ISD       string `json:"ISD"`
	Name      string `json:"Name"`
	Continent string `json:"Cont"`
}

func (c *Country) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}

	type country Country
	switch data[0] {
	case '{':
		var value country
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*c = Country(value)
		return nil
	case '[':
		var values []country
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
		if len(values) > 0 {
			*c = Country(values[0])
		}
		return nil
	default:
		return fmt.Errorf("unsupported Country JSON shape")
	}
}

// New creates a HandyAPI client. The documented free-plan floor is seven
// requests per minute; the shared monthly cap is enforced in PostgreSQL.
func New(apiKey string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		limiter: rate.NewLimiter(rate.Every(time.Minute/7), 2),
	}
}

// Lookup returns nil when HandyAPI explicitly reports that a BIN is absent.
func (c *Client) Lookup(ctx context.Context, bin string) (*Response, error) {
	if err := c.allowRequest(); err != nil {
		return nil, err
	}

	endpoint := c.baseURL + "/" + url.PathEscape(bin)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create HandyAPI request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "beon-bin-api/1.0")
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("HandyAPI request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		c.recordSuccess()
		return nil, nil
	case http.StatusTooManyRequests:
		c.recordFailure()
		return nil, ErrRateLimited
	case http.StatusOK:
		// Continue below.
	default:
		if resp.StatusCode >= http.StatusInternalServerError ||
			resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			c.recordFailure()
		}
		return nil, fmt.Errorf("HandyAPI returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("read HandyAPI response: %w", err)
	}
	var result Response
	if err := json.Unmarshal(body, &result); err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("parse HandyAPI response: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(result.Status), "SUCCESS") {
		c.recordFailure()
		return nil, fmt.Errorf("HandyAPI returned result status %q", result.Status)
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
