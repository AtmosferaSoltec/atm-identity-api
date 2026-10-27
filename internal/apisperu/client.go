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
	DocumentNumber  string
	FirstNames      string
	PaternalSurname string
	MaternalSurname string
}

type Company struct {
	DocumentNumber string
	LegalName      string
	TradeName      string
	Address        string
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
	Nombres         string `json:"nombres"`
	ApellidoPaterno string `json:"apellidoPaterno"`
	ApellidoMaterno string `json:"apellidoMaterno"`
}

type companyResponse struct {
	RazonSocial     string `json:"razonSocial"`
	NombreComercial string `json:"nombreComercial"`
	Direccion       string `json:"direccion"`
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
		DocumentNumber:  documentNumber,
		FirstNames:      r.Nombres,
		PaternalSurname: r.ApellidoPaterno,
		MaternalSurname: r.ApellidoMaterno,
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
	return &Company{
		DocumentNumber: documentNumber,
		LegalName:      r.RazonSocial,
		TradeName:      r.NombreComercial,
		Address:        r.Direccion,
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
