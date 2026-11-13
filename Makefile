COMPOSE = docker compose -f deploy/compose.yaml

# Local builds include the Swagger UI. Production builds (plain go build or the
# Dockerfile without this tag) compile it out entirely.
GO_TAGS = swagger

.DEFAULT_GOAL := help

.PHONY: help up up-upi up-clinic up-upi-clinic up-all up-none down down-v restart build logs ps db db-down migrate db-reset run test vet swagger tidy create-admin cover consumer reminders expire-payments expire-proposals reconcile-refunds purge-sessions expire-calls

help:
	@echo Docker:
	@echo   make up        build and start postgres, migrate and api in the background
	@echo                  payments: razorpay, or PAYMENT_METHODS from the shell
	@echo   make up-upi         same, payments: upi only
	@echo   make up-clinic      same, payments: clinic only
	@echo   make up-upi-clinic  same, payments: upi and clinic, production without Razorpay
	@echo   make up-all         same, payments: razorpay, upi and clinic
	@echo   make up-none        same, payments: none, every booking paid at the clinic
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
	@echo   make reconcile-refunds settle refunds still waiting for the gateway, once
	@echo   make purge-sessions   delete sessions ended more than 90 days ago, once
	@echo   make expire-calls     end call rooms whose time is over, once

# The up-* variants set PAYMENT_METHODS for the api container (deploy/compose.yaml
# passes it through); running one recreates the api with the new methods.
UP_VARIANTS = up-upi up-clinic up-upi-clinic up-all up-none

up-upi: export PAYMENT_METHODS = upi
up-clinic: export PAYMENT_METHODS = clinic
up-upi-clinic: export PAYMENT_METHODS = upi,clinic
up-all: export PAYMENT_METHODS = razorpay,upi,clinic
up-none: export PAYMENT_METHODS =

up $(UP_VARIANTS): export GO_BUILD_TAGS = $(GO_TAGS)
up $(UP_VARIANTS):
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

reconcile-refunds:
	go run ./scripts/cron/reconcile_refunds

purge-sessions:
	go run ./scripts/cron/purge_sessions

expire-calls:
	go run ./scripts/cron/expire_calls
