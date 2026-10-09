package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
)

func (a *App) Routes() http.Handler {
	return api.HandlerWithOptions(a, api.ChiServerOptions{
		BaseRouter: chi.NewRouter(),
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, r, codeInvalidRequest, invalidParamDetail(err))
		},
	})
}
