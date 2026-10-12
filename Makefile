PLUGIN_NAME ?= client-guard
VERSION ?= 0.1.2
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
OUT ?= dist/$(GOOS)/$(GOARCH)/$(PLUGIN_NAME).so

.PHONY: build test check clean
check:
	gofmt -l . ; go vet ./... && go test -race -count=1 ./...
build:
	mkdir -p $(dir $(OUT))
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -buildmode=c-shared -ldflags "-s -w -X main.pluginVersion=$(VERSION)" -o $(OUT) .
	rm -f $(basename $(OUT)).h
test:
	go test ./...
clean:
	rm -rf dist
