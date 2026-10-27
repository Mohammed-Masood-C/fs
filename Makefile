BINARY  ?= fs
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X fs/internal/version.Version=$(VERSION)
BUILD   := go build -trimpath -ldflags "$(LDFLAGS)"

PLATFORMS := windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: build release vet clean

build:
	$(BUILD) -o $(BINARY)$(if $(filter windows,$(shell go env GOOS)),.exe) .

release:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; \
		[ "$$os" = windows ] && ext=".exe"; \
		echo "building dist/$(BINARY)-$$os-$$arch$$ext"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(BUILD) -o dist/$(BINARY)-$$os-$$arch$$ext . || exit 1; \
	done

vet:
	go vet ./...

clean:
	rm -rf $(BINARY) $(BINARY).exe dist
