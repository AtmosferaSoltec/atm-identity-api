package companies

import "time"

// Company is the API representation of a row in companies.
type Company struct {
	DocumentNumber string    `json:"document_number"`
	LegalName      string    `json:"legal_name"`
	TradeName      *string   `json:"trade_name"`
	Address        *string   `json:"address"`
	LookedUpAt     time.Time `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
