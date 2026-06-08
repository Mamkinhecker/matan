DATABASE_URL ?= postgres://postgres:postgres@localhost:5544/matan_trainer?sslmode=disable
GOCACHE ?= /private/tmp/matan-go-build

.PHONY: run test tidy

run:
	DATABASE_URL="$(DATABASE_URL)" GOCACHE="$(GOCACHE)" go run ./cmd/server

test:
	GOCACHE="$(GOCACHE)" go test ./...

tidy:
	GOCACHE="$(GOCACHE)" go mod tidy
