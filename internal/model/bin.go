package model

import "time"

// BIN represents a Bank Identification Number record
type BIN struct {
	ID              int64     `db:"id"               json:"id"`
	BIN             string    `db:"bin"              json:"bin"`
	Brand           string    `db:"brand"            json:"brand"`
	Type            string    `db:"type"             json:"type"`
	Category        string    `db:"category"         json:"category"`
	BankName        string    `db:"bank_name"        json:"bank_name"`
	BankURL         string    `db:"bank_url"         json:"bank_url"`
	BankPhone       string    `db:"bank_phone"       json:"bank_phone"`
	CountryName     string    `db:"country_name"     json:"country_name"`
	CountryCode     string    `db:"country_code"     json:"country_code"`
	CountryCurrency string    `db:"country_currency" json:"country_currency"`
	CountryLatitude float64   `db:"country_latitude" json:"country_latitude"`
	CountryLong     float64   `db:"country_longitude" json:"country_longitude"`
	Prepaid         bool      `db:"prepaid"          json:"prepaid"`
	Source          string    `db:"source"           json:"source"`
	CreatedAt       time.Time `db:"created_at"       json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"       json:"updated_at"`
}

// BINResponse is the public API response shape
type BINResponse struct {
	BIN      string      `json:"bin"`
	Brand    string      `json:"brand"`
	Type     string      `json:"type"`
	Category string      `json:"category"`
	Prepaid  bool        `json:"prepaid"`
	Bank     BankInfo    `json:"bank"`
	Country  CountryInfo `json:"country"`
}

// BankInfo holds issuing bank details
type BankInfo struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Phone string `json:"phone"`
}

// CountryInfo holds country details
type CountryInfo struct {
	Name      string  `json:"name"`
	Code      string  `json:"code"`
	Currency  string  `json:"currency"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// ToResponse converts a BIN record to the public API shape
func (b *BIN) ToResponse() BINResponse {
	return BINResponse{
		BIN:      b.BIN,
		Brand:    b.Brand,
		Type:     b.Type,
		Category: b.Category,
		Prepaid:  b.Prepaid,
		Bank: BankInfo{
			Name:  b.BankName,
			URL:   b.BankURL,
			Phone: b.BankPhone,
		},
		Country: CountryInfo{
			Name:      b.CountryName,
			Code:      b.CountryCode,
			Currency:  b.CountryCurrency,
			Latitude:  b.CountryLatitude,
			Longitude: b.CountryLong,
		},
	}
}
