SHELL := /bin/bash
.DEFAULT_GOAL := help
SQLC_VERSION := 1.31.1
COMPOSE := docker compose

.PHONY: help env demo-env seed-accounts up down logs migrate seed-dev test test-backend test-frontend test-integration test-backup backup generate check compose-check smoke smoke-auth

help:
	@echo 'MedDesk: make env, make up, make seed-accounts, make smoke, make smoke-auth, make test, make test-integration, make backup, make test-backup, make down'

env:
	@bash scripts/init-env.sh

demo-env: env
	@bash scripts/init-demo-env.sh

seed-accounts: demo-env
	$(COMPOSE) build backend
	$(COMPOSE) --profile tools run --rm demo-seed

up: env
	$(COMPOSE) up --build -d --wait --wait-timeout 180

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f --tail=100 backend nginx

migrate:
	$(COMPOSE) run --rm migrate

seed-dev:
	$(COMPOSE) --profile tools run --rm seed

test: test-backend test-frontend

test-backend:
	cd backend && go test -race ./...
	cd backend && go vet ./...

test-frontend:
	cd frontend && npm ci
	cd frontend && npm run typecheck
	cd frontend && npm run test
	cd frontend && npm run build

# Starts an isolated database and removes it afterward; never uses project data.
test-integration:
	@bash scripts/test-integration.sh

backup:
	@bash scripts/backup-dev.sh

test-backup:
	@bash scripts/test-backup.sh

generate:
	docker run --rm --user "$$(id -u):$$(id -g)" -v "$$(pwd)/backend:/src" -w /src sqlc/sqlc:$(SQLC_VERSION) generate

compose-check: env
	$(COMPOSE) config --quiet

check: test compose-check

smoke:
	@bash scripts/smoke.sh

smoke-auth:
	@python3 scripts/smoke-auth.py
