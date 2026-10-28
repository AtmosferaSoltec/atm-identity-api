// Package apisperu is the HTTP client for the APIs Peru service. It is the only
// place that knows the upstream Spanish field names.
package apisperu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

// ErrNotFound means the upstream has no record for the document.
var ErrNotFound = errors.New("apisperu: document not found")

// ErrQuotaExceeded means the provider refused the call because the plan's quota is used up.
var ErrQuotaExceeded = errors.New("apisperu: quota exceeded")

// ErrUnauthorized means the provider rejected our token (invalid, expired or disabled).
var ErrUnauthorized = errors.New("apisperu: unauthorized")

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

	if resp.StatusCode >= 400 {
		return classifyError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("apisperu: decode response: %w", err)
	}
	return nil
}

var quotaWords = regexp.MustCompile(`(?i)l[ií]mite|cuota|excedi|agot|consultas|quota|limit`)

// classifyError maps a failed response to a sentinel error. The provider does not
// document its quota response, so besides 402/429 we also look at the message.
func classifyError(resp *http.Response) error {
	var body struct {
		Message string `json:"message"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	_ = json.Unmarshal(raw, &body)

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusPaymentRequired, resp.StatusCode == http.StatusTooManyRequests,
		quotaWords.MatchString(body.Message):
		return ErrQuotaExceeded
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	default:
		return fmt.Errorf("apisperu: unexpected status %d", resp.StatusCode)
	}
}
