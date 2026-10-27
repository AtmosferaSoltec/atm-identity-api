package persons

import "time"

// Person is the API representation of a row in persons. Unknown values are nil.
type Person struct {
	DocumentNumber    string    `json:"document_number"`
	FirstNames        string    `json:"first_names"`
	PaternalSurname   string    `json:"paternal_surname"`
	MaternalSurname   *string   `json:"maternal_surname"`
	FullName          string    `json:"full_name"`
	VerificationDigit *string   `json:"verification_digit"`
	BirthDate         *string   `json:"birth_date"` // YYYY-MM-DD
	Sex               *string   `json:"sex"`
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
