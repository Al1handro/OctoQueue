.PHONY: up, down, seed

up:
	docker compose up --build

down:
	docker compose down

seed:
	@echo "Seeding database..."
