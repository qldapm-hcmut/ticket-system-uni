SHELL := /bin/bash
export GOTOOLCHAIN ?= auto
-include .env
export

.PHONY: run worker build test test-int lint fmt db-up db-down migrate-up migrate-down sqlc seed
run:
	go run ./cmd/api
worker:
	go run ./cmd/worker
build:
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker
test:
	go test -race ./...
test-int:
	go test -race -tags=integration ./...
lint:
	@test -z "$$(gofmt -l cmd internal)" || (echo 'Run make fmt'; exit 1)
	go vet ./...
fmt:
	gofmt -w cmd internal
db-up:
	docker compose up -d --wait postgres
db-down:
	docker compose down
migrate-up:
	migrate -path db/migrations -database "$(DATABASE_URL)" up
migrate-down:
	migrate -path db/migrations -database "$(DATABASE_URL)" down 1
sqlc:
	sqlc generate
seed:
	@test -n "$(DATABASE_URL)" || (echo 'DATABASE_URL is required'; exit 1)
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f db/seed/seed.sql
