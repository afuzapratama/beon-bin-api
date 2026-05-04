package binlist

import (
"encoding/json"
"fmt"
"io"
"net/http"
"time"
)

const baseURL = "https://lookup.binlist.net"

// Client is an HTTP client for the binlist.net public API
type Client struct {
http    *http.Client
baseURL string
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
Prepaid bool   `json:"prepaid"`
Country struct {
Numeric   string  `json:"numeric"`
Alpha2    string  `json:"alpha2"`
Name      string  `json:"name"`
Emoji     string  `json:"emoji"`
Currency  string  `json:"currency"`
Latitude  float64 `json:"latitude"`
Longitude float64 `json:"longitude"`
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
}
}

// Lookup queries binlist.net for a given BIN (6 to 8 digits)
func (c *Client) Lookup(bin string) (*Response, error) {
url := fmt.Sprintf("%s/%s", c.baseURL, bin)
req, err := http.NewRequest(http.MethodGet, url, nil)
if err != nil {
return nil, err
}
req.Header.Set("Accept-Version", "3")
req.Header.Set("User-Agent", "beon-bin-api/1.0")

resp, err := c.http.Do(req)
if err != nil {
return nil, fmt.Errorf("binlist.net request: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode == http.StatusNotFound {
return nil, nil
}
if resp.StatusCode == http.StatusTooManyRequests {
return nil, fmt.Errorf("binlist.net rate limit hit")
}
if resp.StatusCode != http.StatusOK {
return nil, fmt.Errorf("binlist.net returned status %d", resp.StatusCode)
}

body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
if err != nil {
return nil, err
}

var result Response
if err := json.Unmarshal(body, &result); err != nil {
return nil, fmt.Errorf("parse binlist.net response: %w", err)
}
return &result, nil
}
