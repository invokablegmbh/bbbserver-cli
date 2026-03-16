APP_NAME := bbbserver-cli
PKG := ./cmd/$(APP_NAME)
DIST_DIR := ./dist
VERSION ?= dev
COLLECTION_URL ?= https://documenter.gw.postman.com/api/collections/7658590/T1DwdET1?segregateAuth=true&versionTag=latest
GENERATED_COLLECTION_FILE := internal/postman/collection_gen.go

.PHONY: build test lint install release-dry collection-generate clean

collection-generate:
	@tmp_file=$$(mktemp); \
		curl -fsSL "$(COLLECTION_URL)" -o "$$tmp_file"; \
		python3 ./scripts/generate_collection.py "$$tmp_file" "$(GENERATED_COLLECTION_FILE)"
	@rm -f "$$tmp_file"

build: clean collection-generate
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X bbbserver-cli/internal/version.Version=$(VERSION)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X bbbserver-cli/internal/version.Version=$(VERSION)" -o $(DIST_DIR)/$(APP_NAME)-linux-arm64 $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X bbbserver-cli/internal/version.Version=$(VERSION)" -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe $(PKG)

test: collection-generate
	go test ./...

lint:
	go vet ./...

install: collection-generate
	CGO_ENABLED=0 go install -trimpath -ldflags "-X bbbserver-cli/internal/version.Version=$(VERSION)" $(PKG)

release-dry:
	goreleaser release --snapshot --clean

clean:
	rm -rf dist bin
