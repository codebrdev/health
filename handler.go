package health

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/codebrdev/response"
)

const (
	statusUp   = "up"
	statusDown = "down"

	defaultCheckTimeout = 2 * time.Second
)

// CheckHandler representa uma dependência monitorada (banco, cache, fila...)
type CheckHandler interface {
	Name() string
	Check(ctx context.Context) error
}

type HealthOptions struct {
	Version     string
	Environment string
	StartedAt   time.Time
	Checks      []CheckHandler
	// CheckTimeout limita o tempo de cada check; o padrão é 2s
	CheckTimeout time.Duration
	Logger       *slog.Logger
}

func NewHealthHandler(options HealthOptions) http.HandlerFunc {
	if options.StartedAt.IsZero() {
		options.StartedAt = time.Now()
	}
	if options.CheckTimeout <= 0 {
		options.CheckTimeout = defaultCheckTimeout
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	startedAt := options.StartedAt

	return func(w http.ResponseWriter, r *http.Request) {
		checks, allUp := runChecks(r.Context(), options)

		status := http.StatusOK
		if !allUp {
			status = http.StatusServiceUnavailable
		}

		response.JSON(w, status, map[string]any{
			"version":     options.Version,
			"environment": options.Environment,
			"uptime":      time.Since(startedAt).Round(time.Second).String(),
			"started_at":  startedAt.Format(time.RFC3339),
			"checks":      checks,
		})
	}
}

// runChecks executa todos os checks em paralelo, cada um com seu próprio timeout.
// O detalhe do erro vai só para o log, pois o endpoint é público e poderia vazar host/porta.
func runChecks(ctx context.Context, options HealthOptions) (map[string]string, bool) {
	results := make([]error, len(options.Checks))

	var wg sync.WaitGroup
	for i, check := range options.Checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					results[i] = errPanic
				}
			}()

			checkCtx, cancel := context.WithTimeout(ctx, options.CheckTimeout)
			defer cancel()
			results[i] = check.Check(checkCtx)
		}()
	}
	wg.Wait()

	checks := make(map[string]string, len(options.Checks))
	allUp := true
	for i, check := range options.Checks {
		if err := results[i]; err != nil {
			allUp = false
			checks[check.Name()] = statusDown
			options.Logger.Error("health check falhou", slog.String("check", check.Name()), slog.Any("error", err))
			continue
		}
		checks[check.Name()] = statusUp
	}
	return checks, allUp
}
