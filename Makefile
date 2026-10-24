.PHONY: run test build up migrate seed
run:
	go run -tags swagger ./cmd/http
test:
	go test ./...
	go test -tags swagger ./...
build:
	go build ./...
up:
	docker compose up --build -d
migrate:
	go run ./cmd/migrate up
seed:
	go run ./cmd/seed -posts 100
