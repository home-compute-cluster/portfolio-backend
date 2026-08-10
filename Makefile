GO ?= go
IMAGE ?= portfolio-backend:dev

.PHONY: help fmt fmt-check tidy-check vet test test-integration test-race test-rate-limit-assignment build docker-build ci

help:
	@echo "Available targets: fmt fmt-check tidy-check vet test test-integration test-race test-rate-limit-assignment build docker-build ci"

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal && exit 1)

tidy-check:
	$(GO) mod tidy
	git diff --exit-code -- go.mod go.sum

vet:
	$(GO) vet ./...

test:
	$(GO) test -skip '^TestIntegration' ./...

test-integration:
	$(GO) test -count=1 -run '^TestIntegration' ./...

test-race:
	$(GO) test -race ./...

test-rate-limit-assignment:
	$(GO) test -race -tags assignment ./internal/platform/ratelimit

build:
	mkdir -p bin
	$(GO) build -o bin/api ./cmd/api
	$(GO) build -o bin/migrate ./cmd/migrate

docker-build:
	docker build --tag $(IMAGE) .

ci: fmt-check tidy-check vet test test-race build
