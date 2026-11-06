-include .env
export

MIGRATIONS_DIR := migrations
GOOSE := go tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)"

.PHONY: generate build run test migrate migrate-up migrate-down migrate-status lint test-integration test-mutation

generate:
	go generate ./internal/...
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
	go test -race -count=2 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	go tool cover -func=coverage.out | awk '/internal\/(domain|usecase)\// { print }'

migrate: migrate-up

migrate-up:
	$(GOOSE) up

migrate-down:
	$(GOOSE) down

migrate-status:
	$(GOOSE) status

GOLANGCI_LINT ?= $(shell go env GOPATH)/bin/golangci-lint

lint:
	$(GOLANGCI_LINT) run ./...

test-integration:
	go test -race -count=2 -tags=integration ./internal/repository/...

GO_MUTESTING ?= $(shell go env GOPATH)/bin/go-mutesting

test-mutation:
	GO_MUTESTING="$(GO_MUTESTING)" python3 scripts/mutation.py
