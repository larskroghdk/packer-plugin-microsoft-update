GOPATH := $(shell go env GOPATH | tr '\\' '/')
GOEXE := $(shell go env GOEXE)
PLUGIN_NAME := packer-plugin-microsoft-update
PLUGIN_SOURCE := github.com/larskroghdk/microsoft-update
SDK_VERSION := $(shell go list -m -f '{{.Version}}' github.com/hashicorp/packer-plugin-sdk)
GORELEASER_VERSION := 2.20.0
GOVERSIONINFO_VERSION := 1.7.0
GOLANGCI_LINT_VERSION := 2.15.0
GOVULNCHECK_VERSION := 1.10.0
PACKER_SDC := $(GOPATH)/bin/packer-sdc$(GOEXE)
GORELEASER := $(GOPATH)/bin/goreleaser$(GOEXE)
GOVERSIONINFO := $(GOPATH)/bin/goversioninfo$(GOEXE)
GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint$(GOEXE)
API_VERSION = $(shell go run . describe 2>/dev/null | sed -E 's/.*"api_version":"([^"]+)".*/\1/')

.PHONY: all tools generate build install test test-go test-powershell lint vulncheck release release-snapshot clean

all: generate lint test build

tools:
	go install github.com/hashicorp/packer-plugin-sdk/cmd/packer-sdc@$(SDK_VERSION)
	go install github.com/goreleaser/goreleaser/v2@v$(GORELEASER_VERSION)
	go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v$(GOVERSIONINFO_VERSION)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@v$(GOVULNCHECK_VERSION)

generate:
	PATH="$(GOPATH)/bin:$$PATH" go generate ./...
	rm -rf .web-docs
	mkdir -p docs-partials
	$(PACKER_SDC) renderdocs -src docs/integration -partials docs-partials -dst .web-docs

build:
	go build -trimpath -o $(PLUGIN_NAME)$(GOEXE) .

install: build
	packer plugins install --path $(PLUGIN_NAME)$(GOEXE) $(PLUGIN_SOURCE)

test: test-go test-powershell

test-go:
	go test -count=1 ./...

test-powershell:
	pwsh -NoProfile -File test/Invoke-GuestTests.ps1

lint:
	$(GOLANGCI_LINT) run ./...
	pwsh -NoProfile -File test/Invoke-ScriptAnalysis.ps1

vulncheck:
	govulncheck ./...

release-snapshot: generate
	cd .web-docs && zip -qr ../docs.zip .
	API_VERSION="$(API_VERSION)" $(GORELEASER) release --snapshot --clean --skip=publish,sign

release: generate
	cd .web-docs && zip -qr ../docs.zip .
	API_VERSION="$(API_VERSION)" $(GORELEASER) release --clean

clean:
	rm -rf dist docs.zip $(PLUGIN_NAME) $(PLUGIN_NAME).exe *.syso output-* *.log
