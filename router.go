package health

import (
	"net/http"

	"github.com/codebrdev/middleware"
)

// Register expõe GET /health; as dependências monitoradas entram em options.Checks
func Register(mux *http.ServeMux, options HealthOptions, middlewares ...middleware.Middleware) {
	handler := middleware.Chain(middlewares...)(NewHealthHandler(options))
	mux.Handle("GET /health", handler)
}
