# PlayAnything build helpers. `make build` produces ./bin/playanything for this machine.
MODULE  := github.com/inphaseye172/playanything
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%d)
LDFLAGS := -s -w -X $(MODULE)/internal/version.Version=$(VERSION) -X $(MODULE)/internal/version.Commit=$(COMMIT) -X $(MODULE)/internal/version.Date=$(DATE)
export CGO_ENABLED=0

.PHONY: build test vet cross clean install fmt icon rsrc windows-bundle

ICON_SRC := $(wildcard assets/icon/source.png)

# Build the .ico from the Anchor Point Studio icon (assets/icon/source.png) or the placeholder.
icon:
ifneq ($(ICON_SRC),)
	go run ./tools/mkicon -in $(ICON_SRC) -out assets/icon/playanything.ico -png assets/icon/playanything.png
else
	go run ./tools/mkicon -placeholder -out assets/icon/playanything.ico -png assets/icon/playanything.png
endif

# Embed icon + manifest into the Windows executables.
rsrc: icon
	go run github.com/akavel/rsrc@v0.10.2 -ico assets/icon/playanything.ico -manifest cmd/playanything/playanything.manifest -arch amd64 -o cmd/playanything/rsrc_windows_amd64.syso
	go run github.com/akavel/rsrc@v0.10.2 -ico assets/icon/playanything.ico -manifest cmd/playanything/playanything.manifest -arch arm64 -o cmd/playanything/rsrc_windows_arm64.syso

# Zip playanything.exe with the engine DLL found in libmpv/<arch>/libmpv-2.dll
# (the release workflow downloads it; locally: put the DLL there yourself).
windows-bundle: cross
	@for arch in amd64 arm64; do \
	  if [ -f libmpv/$$arch/libmpv-2.dll ]; then \
	    rm -rf bundle/$$arch && mkdir -p bundle/$$arch && \
	    cp dist/playanything-windows-$$arch.exe bundle/$$arch/playanything.exe && \
	    cp libmpv/$$arch/libmpv-2.dll THIRD_PARTY.md LICENSE bundle/$$arch/ && \
	    (cd bundle/$$arch && zip -q -r ../../dist/playanything-windows-$$arch.zip .) && \
	    echo "bundled dist/playanything-windows-$$arch.zip"; \
	  else echo "no libmpv/$$arch/libmpv-2.dll - skipping $$arch bundle"; fi; \
	done
	cd dist && (command -v sha256sum >/dev/null && sha256sum * || shasum -a 256 *) > SHA256SUMS

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
cross: clean rsrc
	mkdir -p dist
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS) -H windowsgui' -o dist/playanything-windows-amd64.exe ./cmd/playanything
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS) -H windowsgui' -o dist/playanything-windows-arm64.exe ./cmd/playanything
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/playanything-darwin-amd64 ./cmd/playanything
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/playanything-darwin-arm64 ./cmd/playanything
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/playanything-linux-amd64 ./cmd/playanything
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/playanything-linux-arm64 ./cmd/playanything
	cd dist && (command -v sha256sum >/dev/null && sha256sum * || shasum -a 256 *) > SHA256SUMS

# Install for the current user from a local checkout (same layout as install.sh).
install: build
	mkdir -p $(HOME)/.local/bin
	cp bin/playanything $(HOME)/.local/bin/playanything
	$(HOME)/.local/bin/playanything setup

clean:
	rm -rf bin dist bundle
