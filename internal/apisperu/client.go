// Package apisperu is the HTTP client for the APIs Peru service. It is the only
// place that knows the upstream Spanish field names.
package apisperu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ErrNotFound means the upstream has no record for the document.
var ErrNotFound = errors.New("apisperu: document not found")

// ErrTimeout means the upstream did not answer in time.
var ErrTimeout = errors.New("apisperu: timeout")

type Person struct {
	DocumentNumber    string
	FirstNames        string
	PaternalSurname   string
	MaternalSurname   string
	VerificationDigit string
}

type Company struct {
	DocumentNumber    string
	LegalName         string
	TradeName         string
	CompanyType       string
	TaxpayerStatus    string
	TaxpayerCondition string
	Address           string
	District          string
	Province          string
	Department        string
	Ubigeo            string
	Phone             string
}

// Lookup is the port used by the services, so tests can fake the upstream.
type Lookup interface {
	Person(ctx context.Context, documentNumber string) (*Person, error)
	Company(ctx context.Context, documentNumber string) (*Company, error)
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	return &Client{baseURL: baseURL, token: token, http: &http.Client{Timeout: timeout}}
}

type personResponse struct {
	Success          *bool  `json:"success"`
	DNI              string `json:"dni"`
	Nombres          string `json:"nombres"`
	ApellidoPaterno  string `json:"apellidoPaterno"`
	ApellidoMaterno  string `json:"apellidoMaterno"`
	CodVerificaLetra string `json:"codVerificaLetra"`
	CodVerifica      any    `json:"codVerifica"`
}

type companyResponse struct {
	Success         *bool    `json:"success"`
	RUC             string   `json:"ruc"`
	RazonSocial     string   `json:"razonSocial"`
	NombreComercial string   `json:"nombreComercial"`
	Tipo            string   `json:"tipo"`
	Estado          string   `json:"estado"`
	Condicion       string   `json:"condicion"`
	Direccion       string   `json:"direccion"`
	Departamento    string   `json:"departamento"`
	Provincia       string   `json:"provincia"`
	Distrito        string   `json:"distrito"`
	Ubigeo          string   `json:"ubigeo"`
	Telefonos       []string `json:"telefonos"`
}

func (c *Client) Person(ctx context.Context, documentNumber string) (*Person, error) {
	var r personResponse
	if err := c.get(ctx, "dni", documentNumber, &r); err != nil {
		return nil, err
	}
	if r.Nombres == "" {
		return nil, ErrNotFound
	}
	return &Person{
		DocumentNumber:    documentNumber,
		FirstNames:        r.Nombres,
		PaternalSurname:   r.ApellidoPaterno,
		MaternalSurname:   r.ApellidoMaterno,
		VerificationDigit: r.CodVerificaLetra,
	}, nil
}

func (c *Client) Company(ctx context.Context, documentNumber string) (*Company, error) {
	var r companyResponse
	if err := c.get(ctx, "ruc", documentNumber, &r); err != nil {
		return nil, err
	}
	if r.RazonSocial == "" {
		return nil, ErrNotFound
	}
	phone := ""
	if len(r.Telefonos) > 0 {
		phone = r.Telefonos[0]
	}
	return &Company{
		DocumentNumber: documentNumber, LegalName: r.RazonSocial, TradeName: r.NombreComercial,
		CompanyType: r.Tipo, TaxpayerStatus: r.Estado, TaxpayerCondition: r.Condicion,
		Address: r.Direccion, District: r.Distrito, Province: r.Provincia, Department: r.Departamento,
		Ubigeo: r.Ubigeo, Phone: phone,
	}, nil
}

func (c *Client) get(ctx context.Context, kind, documentNumber string, out any) error {
	endpoint := fmt.Sprintf("%s/%s/%s?token=%s", c.baseURL, kind, url.PathEscape(documentNumber), url.QueryEscape(c.token))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var netErr interface{ Timeout() bool }
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return ErrTimeout
		}
		// The URL embeds the token: never wrap the raw transport error.
		return errors.New("apisperu: request failed")
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 400:
		return fmt.Errorf("apisperu: unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("apisperu: decode response: %w", err)
	}
	return nil
}
