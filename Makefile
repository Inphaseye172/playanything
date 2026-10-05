# PlayAnything build helpers. `make build` produces ./bin/playanything for this machine.
MODULE  := github.com/inphaseye172/playanything
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%d)
LDFLAGS := -s -w -X $(MODULE)/internal/version.Version=$(VERSION) -X $(MODULE)/internal/version.Commit=$(COMMIT) -X $(MODULE)/internal/version.Date=$(DATE)
export CGO_ENABLED=0

.PHONY: build test vet cross clean install fmt

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/playanything ./cmd/playanything

test:
	go test ./...

vet:
	go vet ./...
	gofmt -l . | tee /dev/stderr | test -z "$$(cat)"

fmt:
	gofmt -w cmd internal

# Cross-compile every release target into dist/.
cross: clean
	mkdir -p dist
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS) -H windowsgui' -o dist/playanything-windows-amd64.exe ./cmd/playanything
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS) -H windowsgui' -o dist/playanything-windows-arm64.exe ./cmd/playanything
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/playanything-darwin-amd64 ./cmd/playanything
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/playanything-darwin-arm64 ./cmd/playanything
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/playanything-linux-amd64 ./cmd/playanything
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/playanything-linux-arm64 ./cmd/playanything
	cd dist && sha256sum * > SHA256SUMS

# Install for the current user from a local checkout (same layout as install.sh).
install: build
	mkdir -p $(HOME)/.local/bin
	cp bin/playanything $(HOME)/.local/bin/playanything
	$(HOME)/.local/bin/playanything setup

clean:
	rm -rf bin dist
