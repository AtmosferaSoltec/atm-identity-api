package apisperu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "secret-token", 2*time.Second)
}

func TestPersonMapsSpanishFields(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dni/74455518" || r.URL.Query().Get("token") != "secret-token" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"success":true,"dni":"74455518","nombres":"JOEL JOAS","apellidoPaterno":"MALDONADO","apellidoMaterno":"FERNANDEZ"}`))
	})
	p, err := c.Person(context.Background(), "74455518")
	if err != nil {
		t.Fatal(err)
	}
	if p.FirstNames != "JOEL JOAS" || p.PaternalSurname != "MALDONADO" || p.MaternalSurname != "FERNANDEZ" {
		t.Fatalf("bad mapping: %+v", p)
	}
}

func TestCompanyMapsSpanishFields(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ruc":"20131312955","razonSocial":"SUNAT","nombreComercial":"-","direccion":"AV. X 123"}`))
	})
	co, err := c.Company(context.Background(), "20131312955")
	if err != nil {
		t.Fatal(err)
	}
	if co.LegalName != "SUNAT" || co.TradeName != "-" || co.Address != "AV. X 123" {
		t.Fatalf("bad mapping: %+v", co)
	}
}

func TestNotFound(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"http 404":   func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) },
		"empty body": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"success":false}`)) },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := newTestClient(t, handler).Person(context.Background(), "00000000"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("want ErrNotFound, got %v", err)
			}
		})
	}
}

func TestUpstreamErrorDoesNotLeakToken(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	_, err := c.Person(context.Background(), "74455518")
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("want an error without the token, got %v", err)
	}

	dead := NewClient("http://127.0.0.1:1", "secret-token", time.Second)
	_, err = dead.Person(context.Background(), "74455518")
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("transport error must not leak the token, got %v", err)
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) }))
	defer srv.Close()
	_, err := NewClient(srv.URL, "t", 50*time.Millisecond).Person(context.Background(), "74455518")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"invalid token", 401, `{"message":"Token inválido."}`, ErrUnauthorized},
		{"forbidden", 403, `{"message":"Acceso denegado"}`, ErrUnauthorized},
		{"too many requests", 429, `{}`, ErrQuotaExceeded},
		{"payment required", 402, `{}`, ErrQuotaExceeded},
		{"quota message on 401", 401, `{"message":"Has superado el límite de consultas del mes"}`, ErrQuotaExceeded},
		{"quota message on 403", 403, `{"message":"Cuota agotada"}`, ErrQuotaExceeded},
		{"not found", 404, `{}`, ErrNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			})
			if _, err := client.Person(context.Background(), "74455518"); !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
}
