SHELL := /bin/bash

GORELEASER ?= goreleaser
VERSION ?= $(shell git describe --tags --always --dirty)

.PHONY: help build test lint check snapshot release clean

help: ## Show available targets
	@echo "threatoptic-cli"
	@echo ""
	@echo "Develop:"
	@echo "  make build                Build ./threatoptic for this machine"
	@echo "  make test                 Run unit tests"
	@echo "  make lint                 Run go vet and gofmt"
	@echo ""
	@echo "Release:"
	@echo "  make check                Validate .goreleaser.yaml"
	@echo "  make snapshot             Build all platform archives into dist/ without publishing"
	@echo "  make release              Publish the current tag (needs GITHUB_TOKEN)"
	@echo "  make clean                Remove build output"

build: ## Build a binary for the host platform
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o threatoptic ./cmd/threatoptic

test: ## Run unit tests
	go test ./...

lint: ## Run go vet and check formatting
	go vet ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

check: ## Validate the goreleaser configuration
	$(GORELEASER) check

snapshot: ## Build every platform archive locally, without publishing
	$(GORELEASER) release --snapshot --clean --skip=publish

release: ## Publish the release for the current git tag
	$(GORELEASER) release --clean

clean: ## Remove build output
	rm -rf dist threatoptic

.DEFAULT_GOAL := help
