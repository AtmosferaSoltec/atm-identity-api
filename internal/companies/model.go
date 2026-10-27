package companies

import "time"

// Company is the API representation of a row in companies. Unknown values are nil.
type Company struct {
	DocumentNumber    string    `json:"document_number"`
	LegalName         string    `json:"legal_name"`
	TradeName         *string   `json:"trade_name"`
	CompanyType       *string   `json:"company_type"`
	TaxpayerStatus    *string   `json:"taxpayer_status"`
	TaxpayerCondition *string   `json:"taxpayer_condition"`
	Email             *string   `json:"email"`
	Phone             *string   `json:"phone"`
	Address           *string   `json:"address"`
	District          *string   `json:"district"`
	Province          *string   `json:"province"`
	Department        *string   `json:"department"`
	Ubigeo            *string   `json:"ubigeo"`
	LookupSource      string    `json:"lookup_source"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
