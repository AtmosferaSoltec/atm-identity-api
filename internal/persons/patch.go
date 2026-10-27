package persons

import (
	"atm-identity-api/internal/enrichment"
	"atm-identity-api/internal/platform/apierror"
)

// PatchInput holds the fields a client system may enrich. Omitted and null
// fields are ignored: a PATCH never erases data.
type PatchInput struct {
	BirthDate  *string `json:"birth_date"`
	Sex        *string `json:"sex"`
	Email      *string `json:"email"`
	Phone      *string `json:"phone"`
	Address    *string `json:"address"`
	District   *string `json:"district"`
	Province   *string `json:"province"`
	Department *string `json:"department"`
	Ubigeo     *string `json:"ubigeo"`
}

var target = enrichment.Target{
	Table:      "persons",
	EntityType: "person",
	Columns: map[string]string{
		"birth_date": "date", "sex": "", "email": "", "phone": "", "address": "",
		"district": "", "province": "", "department": "", "ubigeo": "",
	},
}

func text(s string) string { return enrichment.Text(s, 255) }

func (in PatchInput) validate() []apierror.Detail {
	return enrichment.Collect(
		enrichment.Run("birth_date", in.BirthDate, enrichment.BirthDate),
		enrichment.Run("sex", in.Sex, enrichment.Sex),
		enrichment.Run("email", in.Email, enrichment.Email),
		enrichment.Run("phone", in.Phone, enrichment.Phone),
		enrichment.Run("address", in.Address, text),
		enrichment.Run("district", in.District, text),
		enrichment.Run("province", in.Province, text),
		enrichment.Run("department", in.Department, text),
		enrichment.Run("ubigeo", in.Ubigeo, enrichment.Ubigeo),
	)
}

func (in PatchInput) updates() map[string]string {
	out := map[string]string{}
	for column, v := range map[string]*string{
		"birth_date": in.BirthDate, "sex": in.Sex, "email": in.Email, "phone": in.Phone,
		"address": in.Address, "district": in.District, "province": in.Province,
		"department": in.Department, "ubigeo": in.Ubigeo,
	} {
		if v != nil {
			out[column] = *v
		}
	}
	return out
}
