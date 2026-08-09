GO ?= go
IMAGE ?= portfolio-backend:dev

.PHONY: help fmt fmt-check tidy-check vet test test-race build docker-build ci

help:
	@echo "Available targets: fmt fmt-check tidy-check vet test test-race build docker-build ci"

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
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

build:
	$(GO) build -o bin/api ./cmd/api

docker-build:
	docker build --tag $(IMAGE) .

ci: fmt-check tidy-check vet test test-race build
