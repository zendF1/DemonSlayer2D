# Local settings (see .env.example). Every variable in .env is exported to the
# commands below; the defaults here apply only when .env does not set them.
-include .env
export

DATABASE_URL ?= postgres://ds:ds@localhost:5433/ds?sslmode=disable
TEST_DATABASE_URL ?= postgres://ds:ds@localhost:5433/ds_test?sslmode=disable

.PHONY: proto db-up db-down run vet test test-db

proto:
	go tool buf generate

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

run:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/gameserver

vet:
	go vet ./...

test:
	go test -race ./...

test-db:
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -race ./...
