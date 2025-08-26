# Variables
GO_BUILD_FLAGS := -ldflags="-s -w"

GO_BIN := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

# Build targets
.PHONY: build arm64 amd64

.DEFAULT_GOAL := build

build: arm64 amd64

arm64:
	@echo "Building for arm64..."
	mkdir -p build
	rm -rf accel-exporter.arm64
	cd cmd/accel-exporter && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -o ../../build/accel-exporter.arm64

amd64:
	@echo "Building for amd64..."
	mkdir -p build
	rm -rf accel-exporter.amd64
	cd cmd/accel-exporter && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -o ../../build/accel-exporter.amd64

