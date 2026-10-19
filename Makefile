.PHONY: deps db.up db.down run dev test vet migrate.up migrate.down migrate.status migrate.reset docker.build migrate.create sqlc seed

deps:
	go mod tidy

db.up:
	docker compose up -d postgres

db.down:
	docker compose down

run:
	go run ./cmd/api

dev:
	air

test:
	go test ./... -race -count=1

vet:
	go vet ./...

migrate.up:
	go run ./cmd/migrate -dir db/migrations up

migrate.down:
	go run ./cmd/migrate -dir db/migrations down

migrate.status:
	go run ./cmd/migrate -dir db/migrations status

migrate.reset:
	go run ./cmd/migrate -dir db/migrations reset

migrate.create:
	goose -s -dir db/migrations create $(name) sql

sqlc:
	sqlc generate

seed:
	go run ./cmd/seed

docker.build:
	docker build -t savannah-hmis:local .
