package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codebrdev/middleware"
)

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("resposta não é um JSON válido: %v", err)
	}
	return body
}

func TestNewHealthHandler(t *testing.T) {
	startedAt := time.Now().Add(-90 * time.Second)
	handler := NewHealthHandler(HealthOptions{Version: "1.2.3", Environment: "test", StartedAt: startedAt})

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	body := decodeBody(t, rec)
	if body["version"] != "1.2.3" {
		t.Errorf("version = %v", body["version"])
	}
	if body["environment"] != "test" {
		t.Errorf("environment = %v", body["environment"])
	}
	if body["uptime"] != "1m30s" {
		t.Errorf("uptime = %v, esperado 1m30s", body["uptime"])
	}
	if body["started_at"] != startedAt.Format(time.RFC3339) {
		t.Errorf("started_at = %v, esperado %s", body["started_at"], startedAt.Format(time.RFC3339))
	}
}

func TestNewHealthHandlerDefaultsStartedAt(t *testing.T) {
	before := time.Now().Add(-time.Second)
	handler := NewHealthHandler(HealthOptions{})

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	body := decodeBody(t, rec)
	startedAt, err := time.Parse(time.RFC3339, body["started_at"].(string))
	if err != nil {
		t.Fatalf("started_at inválido: %v", err)
	}
	if startedAt.Before(before) || startedAt.After(time.Now().Add(time.Second)) {
		t.Errorf("started_at deveria ser aproximadamente agora, obteve %v", startedAt)
	}
	if body["uptime"] != "0s" {
		t.Errorf("uptime = %v, esperado 0s", body["uptime"])
	}
}

func TestNewHealthHandlerUptimeGrows(t *testing.T) {
	handler := NewHealthHandler(HealthOptions{StartedAt: time.Now().Add(-time.Hour)})

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := decodeBody(t, rec)["uptime"]; got != "1h0m0s" {
		t.Errorf("uptime = %v, esperado 1h0m0s", got)
	}
}

func TestRegister(t *testing.T) {
	t.Run("expõe GET /health", func(t *testing.T) {
		mux := http.NewServeMux()
		Register(mux, HealthOptions{Version: "9.9.9", Environment: "staging", StartedAt: time.Now()})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
		}
		body := decodeBody(t, rec)
		if body["version"] != "9.9.9" || body["environment"] != "staging" {
			t.Errorf("config não propagada: %v", body)
		}
	})

	t.Run("rejeita outros métodos", func(t *testing.T) {
		mux := http.NewServeMux()
		Register(mux, HealthOptions{Version: "9.9.9", Environment: "staging", StartedAt: time.Now()})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/health", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, esperado %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("aplica os middlewares na ordem informada", func(t *testing.T) {
		var order []string
		mark := func(name string) middleware.Middleware {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					order = append(order, name)
					next.ServeHTTP(w, r)
				})
			}
		}

		mux := http.NewServeMux()
		Register(mux, HealthOptions{Version: "9.9.9", Environment: "staging", StartedAt: time.Now()}, mark("primeiro"), mark("segundo"))

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

		if len(order) != 2 || order[0] != "primeiro" || order[1] != "segundo" {
			t.Errorf("ordem dos middlewares = %v", order)
		}
	})
}
