VERSION ?= $(shell cat VERSION 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test vet install version release clean

build:
	go build $(LDFLAGS) -o bin/goat .

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

install:
	go install $(LDFLAGS) .

version:
	@echo $(VERSION)

# Cross-compile release binaries into dist/ as tarballs.
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
	@ls -la dist

clean:
	rm -rf dist bin
