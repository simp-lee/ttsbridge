.DEFAULT_GOAL := check

.PHONY: build test race cover lint check-fmt fmt vet vuln arch check tidy

GO ?= go
GOENV_GOBIN := $(strip $(shell $(GO) env GOBIN))
GOENV_GOPATH := $(strip $(shell $(GO) env GOPATH))
GO_BIN_DIR := $(if $(GOENV_GOBIN),$(GOENV_GOBIN),$(GOENV_GOPATH)/bin)
export PATH := $(GO_BIN_DIR):$(PATH)
PKGS ?= ./...
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
COVERPROFILE ?= coverage.out
COVERMODE ?= atomic
ARCH_TEST_PATTERN ?= Test(PublicPackageInventory_RemainsMinimalAndPurposeNamed|CLIEntryPoint_RemainsThinAndDelegatesToInternalCLI|CLIProviderAwareness_StaysConfinedToAdapterFiles|CLIContracts_ReuseUnifiedTTSContracts|PublicAPI_.*|CoreTTSPackage_RemainsProviderAgnostic)

build:
	$(GO) build $(PKGS)

test:
	$(GO) test -count=1 $(PKGS)

race:
	$(GO) test -race -count=1 $(PKGS)

cover:
	$(GO) test -count=1 -covermode=$(COVERMODE) -coverprofile=$(COVERPROFILE) $(PKGS)
	$(GO) tool cover -func=$(COVERPROFILE)

check-fmt:
	$(GOLANGCI_LINT) fmt --diff

fmt:
	$(GOLANGCI_LINT) fmt

tidy:
	$(GO) mod tidy

vet:
	$(GO) vet $(PKGS)

vuln:
	$(GOVULNCHECK) $(PKGS)

lint:
	$(GOLANGCI_LINT) run $(PKGS)

arch:
	$(GO) test -count=1 ./tts -run '$(ARCH_TEST_PATTERN)'

check: build check-fmt arch lint race