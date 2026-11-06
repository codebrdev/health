package health

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestPingCheck(t *testing.T) {
	check := PingCheck("redis", func(context.Context) error { return nil })
	if check.Name() != "redis" {
		t.Errorf("nome inesperado: %s", check.Name())
	}
	if err := check.Check(context.Background()); err != nil {
		t.Fatalf("ping ok deveria passar: %v", err)
	}

	failing := PingCheck("redis", func(context.Context) error { return errors.New("fora") })
	if err := failing.Check(context.Background()); err == nil {
		t.Error("ping com erro deveria falhar")
	}
}

func TestPingCheckNilPing(t *testing.T) {
	if err := PingCheck("redis", nil).Check(context.Background()); err == nil {
		t.Error("ping nil deveria falhar sem panic")
	}
}

func TestSQLCheck(t *testing.T) {
	check := SQLCheck("mysql", nil)
	if check.Name() != "mysql" {
		t.Errorf("Name() = %q", check.Name())
	}
	if err := check.Check(context.Background()); err == nil {
		t.Error("db nil deveria falhar")
	}
}

func TestCheckFunc(t *testing.T) {
	want := errors.New("falhou")
	check := CheckFunc("custom", func(context.Context) error { return want })
	if check.Name() != "custom" || check.Check(context.Background()) != want {
		t.Error("CheckFunc deveria repassar nome e erro")
	}
}

func serveHealth(t *testing.T, opts HealthOptions) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	if opts.Logger == nil {
		opts.Logger = discardLogger()
	}
	rec := httptest.NewRecorder()
	NewHealthHandler(opts)(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	return rec, decodeBody(t, rec)
}

func TestHealthChecksAllUp(t *testing.T) {
	rec, body := serveHealth(t, HealthOptions{Checks: []CheckHandler{
		CheckFunc("mysql", func(context.Context) error { return nil }),
		CheckFunc("redis", func(context.Context) error { return nil }),
	}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", rec.Code)
	}
	checks := body["checks"].(map[string]any)
	if checks["mysql"] != "up" || checks["redis"] != "up" {
		t.Errorf("checks = %v", checks)
	}
}

func TestHealthChecksOneDown(t *testing.T) {
	rec, body := serveHealth(t, HealthOptions{Checks: []CheckHandler{
		CheckFunc("mysql", func(context.Context) error { return nil }),
		CheckFunc("redis", func(context.Context) error { return errors.New("dial tcp 10.0.0.5:6379: refused") }),
	}})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, esperado 503", rec.Code)
	}
	checks := body["checks"].(map[string]any)
	if checks["mysql"] != "up" || checks["redis"] != "down" {
		t.Errorf("checks = %v", checks)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Error("o detalhe do erro não pode vazar na resposta pública")
	}
}

func TestHealthChecksErrorIsLogged(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	serveHealth(t, HealthOptions{Logger: logger, Checks: []CheckHandler{
		CheckFunc("redis", func(context.Context) error { return errors.New("conexão recusada") }),
	}})

	if !strings.Contains(buf.String(), "redis") || !strings.Contains(buf.String(), "conexão recusada") {
		t.Errorf("o erro deveria ir para o log: %s", buf.String())
	}
}

func TestHealthChecksRunInParallelWithTimeout(t *testing.T) {
	var finished atomic.Int32
	slow := func(name string) CheckHandler {
		return CheckFunc(name, func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				finished.Add(1)
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		})
	}

	start := time.Now()
	rec, body := serveHealth(t, HealthOptions{
		CheckTimeout: 100 * time.Millisecond,
		Checks:       []CheckHandler{slow("a"), slow("b"), slow("c")},
	})
	elapsed := time.Since(start)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, esperado 503", rec.Code)
	}
	if elapsed > time.Second {
		t.Errorf("checks deveriam rodar em paralelo e respeitar o timeout, levou %v", elapsed)
	}
	if finished.Load() != 3 {
		t.Errorf("todos os checks deveriam ser cancelados pelo timeout, cancelados: %d", finished.Load())
	}
	for _, name := range []string{"a", "b", "c"} {
		if body["checks"].(map[string]any)[name] != "down" {
			t.Errorf("check %s deveria estar down", name)
		}
	}
}

func TestHealthCheckPanicIsDown(t *testing.T) {
	rec, body := serveHealth(t, HealthOptions{Checks: []CheckHandler{
		CheckFunc("boom", func(context.Context) error { panic("x") }),
		CheckFunc("ok", func(context.Context) error { return nil }),
	}})

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, esperado 503", rec.Code)
	}
	checks := body["checks"].(map[string]any)
	if checks["boom"] != "down" || checks["ok"] != "up" {
		t.Errorf("checks = %v", checks)
	}
}

func TestHealthWithoutChecksIsUp(t *testing.T) {
	rec, body := serveHealth(t, HealthOptions{})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, esperado 200", rec.Code)
	}
	if len(body["checks"].(map[string]any)) != 0 {
		t.Errorf("checks deveria ser vazio: %v", body["checks"])
	}
}
