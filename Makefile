BIN := bin
APP := $(BIN)/baybar.app
GO  ?= go

# Where "make install" copies the built files. Override PREFIX to install
# somewhere else, for example PREFIX=$$HOME/.local.
PREFIX  ?= /usr/local
APPDIR  ?= /Applications

OUT ?= /tmp/baybar.png
REGION ?=

.PHONY: all check clean test e2e docs-check docs-generate install uninstall window
all: $(BIN)/bay $(BIN)/bayd $(APP)

$(BIN)/bay: $(shell find cmd/bay internal \( -name '*.go' -o -name '*.c' -o -name '*.h' \))
	$(GO) build -o $@ ./cmd/bay

$(BIN)/bayd: $(shell find cmd/bayd internal \( -name '*.go' -o -name '*.c' -o -name '*.h' \))
	$(GO) build -o $@ ./cmd/bayd

# The menu bar app has to be a bundle: LSUIElement is what keeps it out of the
# Dock and the app switcher, and only Info.plist can say so.
$(APP): $(BIN)/bay $(BIN)/bayd $(shell find cmd/baybar -type f) $(shell find internal \( -name '*.go' -o -name '*.c' -o -name '*.h' \))
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

# Installs the built files for use outside this directory. bay and
# bayd are installed side by side, which is what the machine setup
# needs: it registers the DNS agent with the path of the bayd beside
# bay.
install: all
	mkdir -p "$(PREFIX)/bin"
	# A binary overwritten in place keeps the file the kernel validated, and the
	# kernel kills the new contents with SIGKILL; a new file is validated afresh.
	rm -f "$(PREFIX)/bin/bay" "$(PREFIX)/bin/bayd"
	cp $(BIN)/bay $(BIN)/bayd "$(PREFIX)/bin/"
	rm -rf "$(APPDIR)/baybar.app"
	cp -R $(APP) "$(APPDIR)/"
	@echo
	@echo "installed $(PREFIX)/bin/bay, $(PREFIX)/bin/bayd"
	@echo "installed $(APPDIR)/baybar.app"
	@echo
	@echo "next: bay install   # one-time machine setup, asks for the password"

uninstall:
	rm -f "$(PREFIX)/bin/bay" "$(PREFIX)/bin/bayd"
	rm -rf "$(APPDIR)/baybar.app"
	@echo "removed the installed files; run \"bay uninstall\" first to undo the machine setup"

# window captures the running window and reports where its rows of type sit, so
# a change to the layout is measured rather than judged by eye. REGION is the
# left, right, top and bottom of the area to measure.
window:
	swift tools/window.swift $(OUT) $(REGION)

clean:
	rm -rf $(BIN)
