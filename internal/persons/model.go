package persons

import "time"

// Person is the API representation of a row in persons.
type Person struct {
	DocumentNumber  string    `json:"document_number"`
	FirstNames      string    `json:"first_names"`
	PaternalSurname string    `json:"paternal_surname"`
	MaternalSurname *string   `json:"maternal_surname"`
	FullName        string    `json:"full_name"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
