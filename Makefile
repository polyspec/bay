BIN := bin
APP := $(BIN)/baybar.app
GO  ?= go

.PHONY: all check clean test e2e docs-check docs-generate install
all: $(BIN)/bay $(BIN)/bayd $(APP)

$(BIN)/bay: $(shell find cmd/bay internal -name '*.go')
	$(GO) build -o $@ ./cmd/bay

$(BIN)/bayd: $(shell find cmd/bayd internal -name '*.go')
	$(GO) build -o $@ ./cmd/bayd

# The menu bar app has to be a bundle: LSUIElement is what keeps it out of the
# Dock and the app switcher, and only Info.plist can say so.
$(APP): $(BIN)/bay $(BIN)/bayd $(shell find cmd/baybar -type f) $(shell find internal -name '*.go')
	mkdir -p $(APP)/Contents/MacOS
	$(GO) build -o $(APP)/Contents/MacOS/baybar ./cmd/baybar
	printf '%s\n' \
	  '<?xml version="1.0" encoding="UTF-8"?>' \
	  '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
	  '<plist version="1.0"><dict>' \
	  '  <key>CFBundleName</key><string>baybar</string>' \
	  '  <key>CFBundleIdentifier</key><string>dev.bay.bar</string>' \
	  '  <key>CFBundleExecutable</key><string>baybar</string>' \
	  '  <key>CFBundlePackageType</key><string>APPL</string>' \
	  '  <key>CFBundleShortVersionString</key><string>0.1</string>' \
	  '  <key>LSUIElement</key><true/>' \
	  '  <key>LSMinimumSystemVersion</key><string>13.0</string>' \
	  '</dict></plist>' > $(APP)/Contents/Info.plist
	plutil -lint $(APP)/Contents/Info.plist >/dev/null
	@# baybar hands the privileged steps to the bay beside it, and
	@# registers a launchd job pointing at the bayd beside it.
	cp $(BIN)/bay $(BIN)/bayd $(APP)/Contents/MacOS/

check: all test docs-check
	$(GO) vet ./...
	@test -z "$$(gofmt -l cmd internal)" || { gofmt -l cmd internal; exit 1; }

test:
	$(GO) test ./...

docs-check: $(BIN)/bay
	$(GO) run ./cmd/docsgen
	$(GO) run ./cmd/docscheck

docs-generate: $(BIN)/bay
	$(GO) run ./cmd/docsgen -write

# Starts real containers.
e2e:
	BAY_E2E=1 $(GO) test ./... -count=1 -timeout 25m

# One-time machine setup. Everything after this runs unprivileged.
install: all
	$(BIN)/bay install

clean:
	rm -rf $(BIN)
