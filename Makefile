COMPOSE = docker compose -f deploy/compose.yaml

.DEFAULT_GOAL := help

.PHONY: help up down restart build logs ps db db-down migrate db-reset run test vet swagger tidy

help:
	@echo Docker:
	@echo   make up        build and start postgres, migrate and api in the background
	@echo   make down      stop and remove the containers
	@echo   make restart   down, then up
	@echo   make build     build the api image only
	@echo   make logs      follow the api logs
	@echo   make ps        list the containers
	@echo   make db        start postgres and apply migrations only
	@echo   make db-down   same as d~own
	@echo   make migrate   apply pending migrations
	@echo   make db-reset  remove the containers and the database volume, then start db again
	@echo Go:
	@echo   make run       go run . against the local database
	@echo   make test      go test ./...
	@echo   make vet       go vet ./...
	@echo   make swagger   regenerate docs/swagger
	@echo   make tidy      go mod tidy

up:
	$(COMPOSE) up --build -d --wait

down:
	$(COMPOSE) down

restart: down up

build:
	$(COMPOSE) build api

logs:
	$(COMPOSE) logs -f api

ps:
	$(COMPOSE) ps

db:
	$(COMPOSE) up -d --wait postgres migrate

db-down: down

migrate:
	$(COMPOSE) run --rm migrate

db-reset:
	$(COMPOSE) down -v
	$(MAKE) db

run:
	go run .

test:
	go test ./...

vet:
	go vet ./...

swagger:
	go generate ./...

tidy:
	go mod tidy
