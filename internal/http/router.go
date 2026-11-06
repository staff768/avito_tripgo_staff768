package transport

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	domain "github.com/staff768/avito_tripgo_staff768/internal/domain/trip"
	api "github.com/staff768/avito_tripgo_staff768/internal/generated"
	"github.com/staff768/avito_tripgo_staff768/internal/http/health"
	"github.com/staff768/avito_tripgo_staff768/internal/http/response"
	"github.com/staff768/avito_tripgo_staff768/internal/http/trip/create"
	"github.com/staff768/avito_tripgo_staff768/internal/http/trip/finish"
	"github.com/staff768/avito_tripgo_staff768/internal/http/trip/get"
)

type API struct {
	api.Unimplemented
	create *create.Handler
	get    *get.Handler
	finish *finish.Handler
	health *health.Handler
}

func New(create *create.Handler, get *get.Handler, finish *finish.Handler, health *health.Handler, logger *slog.Logger) http.Handler {
	server := &API{create: create, get: get, finish: finish, health: health}
	return api.HandlerWithOptions(server, api.ChiServerOptions{BaseRouter: chi.NewRouter(), ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
		response.Error(w, r, logger, &domain.ValidationError{Field: "tripId", Reason: "has an invalid format"})
	}})
}

func (a *API) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	a.create.Handle(w, r, params)
}

func (a *API) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) { a.get.Handle(w, r, id) }

func (a *API) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	a.finish.Handle(w, r, id)
}
func (a *API) Health(w http.ResponseWriter, r *http.Request) { a.health.Health(w, r) }
func (a *API) Ready(w http.ResponseWriter, r *http.Request)  { a.health.Ready(w, r) }
