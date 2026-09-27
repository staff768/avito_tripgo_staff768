-include .env
export

MIGRATIONS_DIR := migrations
GOOSE := go tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)"

.PHONY: generate build run test migrate migrate-up migrate-down migrate-status

generate:
	go tool oapi-codegen \
	-generate types,chi-server \
	-package api \
	-o internal/generated/api.gen.go \
	contracts/openapi/trip-service.openapi.yaml

build:
	go build -o bin/trip-service ./cmd/trip-service

run:
	go run ./cmd/trip-service

test:
	go test -race ./...

migrate: migrate-up

migrate-up:
	$(GOOSE) up

migrate-down:
	$(GOOSE) down

migrate-status:
	$(GOOSE) status
