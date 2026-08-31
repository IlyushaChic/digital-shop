.PHONY: migrate-up migrate-down seed run

DB_URL=postgres://postgres:postgres@localhost:5433/shop?sslmode=disable

migrate-up:
	migrate -database "$(DB_URL)" -path migrations up

migrate-down:
	migrate -database "$(DB_URL)" -path migrations down

seed:
	psql -d shop -U postgres -f seed.sql

run:
	go run cmd/api/main.go

create-db:
	createdb -U postgres shop || true