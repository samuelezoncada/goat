VERSION ?= $(shell cat VERSION 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test test-race vet fmt fmtcheck check fuzz install version release clean

build:
	go build $(LDFLAGS) -o bin/goat .

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Fail if anything is unformatted, printing the offending files.
fmtcheck:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

# Everything CI runs, in one target.
check: fmtcheck vet test-race

fuzz:
	go test ./editor/ -run FuzzBufferEdits -fuzz FuzzBufferEdits -fuzztime 60s

install:
	go install $(LDFLAGS) .

version:
	@echo $(VERSION)

# Cross-compile release binaries into dist/ as tarballs, with checksums.
release:
	@mkdir -p dist
	@set -e; for target in \
		"linux amd64 linux_amd64" \
		"linux arm64 linux_arm64" \
		"darwin amd64 darwin_amd64" \
		"darwin arm64 darwin_arm64" \
		"windows amd64 windows_amd64.exe" \
		"windows arm64 windows_arm64.exe"; do \
		set -- $$target; \
		os=$$1; arch=$$2; name=$$3; \
		GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o dist/goat-$$name .; \
		tar -C dist -czf dist/goat-$${name%.exe}.tar.gz goat-$$name; \
		rm dist/goat-$$name; \
	done
	@cd dist && shasum -a 256 *.tar.gz > SHA256SUMS 2>/dev/null || sha256sum *.tar.gz > SHA256SUMS
	@ls -la dist

clean:
	rm -rf dist bin
