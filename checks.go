package health

import (
	"context"
	"database/sql"
	"errors"
)

var errPanic = errors.New("o check entrou em pânico")

type checkFunc struct {
	name string
	fn   func(ctx context.Context) error
}

func (c checkFunc) Name() string                    { return c.name }
func (c checkFunc) Check(ctx context.Context) error { return c.fn(ctx) }

// CheckFunc adapta uma função qualquer para um CheckHandler (útil para dependências sem checker pronto)
func CheckFunc(name string, fn func(ctx context.Context) error) CheckHandler {
	return checkFunc{name: name, fn: fn}
}

// PingCheck monitora qualquer dependência que exponha um ping (Redis, broker, ...) sem acoplar a lib ao client
func PingCheck(name string, ping func(ctx context.Context) error) CheckHandler {
	return CheckFunc(name, func(ctx context.Context) error {
		if ping == nil {
			return errors.New("ping não configurado")
		}
		return ping(ctx)
	})
}

// SQLCheck verifica a conectividade com um banco SQL (MySQL, Postgres, ...) via PingContext
func SQLCheck(name string, db *sql.DB) CheckHandler {
	return CheckFunc(name, func(ctx context.Context) error {
		if db == nil {
			return errors.New("conexão sql não configurada")
		}
		return db.PingContext(ctx)
	})
}
