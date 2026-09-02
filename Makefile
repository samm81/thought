# https://tech.davis-hansson.com/p/make/
SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

# ANSI color codes
GREEN := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
RED := $(shell tput -Txterm setaf 1)
BLUE := $(shell tput -Txterm setaf 6)
RESET := $(shell tput -Txterm sgr0)

GO ?= go
ARGS ?=
BINARY_NAME ?= thought
BIN_DIR ?= bin

FILES_GO := $(shell find . -type f -name '*.go' -not -path './third_party/*' -print)
FILES_BUILD := $(FILES_GO) Makefile go.mod $(wildcard go.sum)
BINARY_PATH := $(BIN_DIR)/$(BINARY_NAME)

.DEFAULT_GOAL := help

.PHONY: help build run test vet lint format format-check tidy check audit clean

help: ## Show available targets
	@printf '%s\n' 'targets:'
	@grep -E '^[[:alnum:]_./-]+:.*## .+$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  %-18s %s\n", $$1, $$2}'

build: $(BINARY_PATH) ## Build the thought binary

$(BINARY_PATH): $(FILES_BUILD) | $(BIN_DIR)
	$(GO) build -trimpath -o "$@" ./cmd/thought

$(BIN_DIR):
	mkdir -p "$@"

run: $(BINARY_PATH) ## Run thought, optionally passing ARGS="..."
	"$(BINARY_PATH)" $(ARGS)

test: ## Run the test suite with race detection
	$(GO) test -race -shuffle=on ./...

vet: ## Run the standard-library static checks
	$(GO) vet ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

format: ## Format Go source files
	$(GO) fmt ./...

format-check: ## Check that Go source files are formatted
	@files="$$(gofmt -l $(FILES_GO))"
	if [[ -n "$$files" ]]; then
		printf '%s\n' "$$files"
		exit 1
	fi

tidy: ## Tidy Go module dependencies
	$(GO) mod tidy

check: format-check vet test ## Run formatting, vet, and tests

audit: ## Scan reachable dependencies for known vulnerabilities
	govulncheck ./...

clean: ## Remove local build artifacts
	case "/$(BIN_DIR)/" in
		//*|*/../*|*/./*)
			printf '%s\n' 'BIN_DIR must be a safe relative path without . or .. components' >&2
			exit 1
			;;
	esac
	rm -rf -- "$(BIN_DIR)"
