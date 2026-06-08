.PHONY: state, up, down, seed, test, all

all: up start 

start:
	CONFIG_PATH=./config/local.yaml go run main.go

up:
	docker compose up --build -d
	# TODO: Доавить контейнер с веб сервисом или в docker compose web сервер

down:
	docker compose down

seed:
	@echo "Seeding database..."
	docker compose exec -T db psql -U postgres -d app < ./seed.sql
	@echo "Database seeded successfully."
	# TODO: Понадобиться для тестов воркеров

test:
	go test ./... -v
