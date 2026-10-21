COMPOSE = docker compose -f deploy/compose.yaml

# Local builds include the Swagger UI. Production builds (plain go build or the
# Dockerfile without this tag) compile it out entirely.
GO_TAGS = swagger

.DEFAULT_GOAL := help

.PHONY: help up down down-v restart build logs ps db db-down migrate db-reset run test vet swagger tidy create-admin cover consumer reminders expire-payments expire-proposals

help:
	@echo Docker:
	@echo   make up        build and start postgres, migrate and api in the background
	@echo   make down      stop and remove the containers
	@echo   make down-v    stop and remove the containers and volumes, deletes database data
	@echo   make restart   down, then up
	@echo   make build     build the api image only
	@echo   make logs      follow the api logs
	@echo   make ps        list the containers
	@echo   make db        start postgres and apply migrations only
	@echo   make db-down   same as down
	@echo   make migrate   apply pending migrations
	@echo   make db-reset  remove the containers and the database volume, then start db again
	@echo Go:
	@echo   make run       go run . against the local database
	@echo   make test      go test ./...
	@echo   make vet       go vet ./...
	@echo   make cover     go test -coverpkg=./... ./tests/... with a coverage report
	@echo   make swagger   regenerate docs/swagger
	@echo   make tidy      go mod tidy
	@echo Admin:
	@echo   make create-admin  NAME=... EMAIL=... MOBILE=..., first admin only, password from ADMIN_PASSWORD
	@echo Jobs:
	@echo   make consumer  publish outbox events until stopped (Ctrl+C)
	@echo   make reminders queue due appointment reminders once
	@echo   make expire-payments  cancel bookings not paid within the hold, once
	@echo   make expire-proposals close reschedule proposals nobody answered in time, once

up: export GO_BUILD_TAGS = $(GO_TAGS)
up:
	$(COMPOSE) up --build -d --wait

down:
	$(COMPOSE) down

down-v:
	$(COMPOSE) down -v

restart: down up

build: export GO_BUILD_TAGS = $(GO_TAGS)
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
	go run -tags $(GO_TAGS) .

test:
	go test ./...
	go test -tags $(GO_TAGS) ./...

vet:
	go vet ./...
	go vet -tags $(GO_TAGS) ./...

cover:
	go test -tags $(GO_TAGS) -coverpkg=./... ./tests/... -coverprofile=coverage.out
	go tool cover -func=coverage.out

swagger:
	go generate ./...

tidy:
	go mod tidy

create-admin:
	go run ./scripts/bootstrap -name "$(NAME)" -email "$(EMAIL)" -mobile "$(MOBILE)"

consumer:
	go run ./scripts/consumer/outbox

reminders:
	go run ./scripts/cron/reminders

expire-payments:
	go run ./scripts/cron/expire_payments

expire-proposals:
	go run ./scripts/cron/expire_proposals
