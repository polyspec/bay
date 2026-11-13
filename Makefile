BIN := bin
APP := $(BIN)/baybar.app
GO  ?= go

# Where "make install" copies the built files. Override PREFIX to install
# somewhere else, for example PREFIX=$$HOME/.local.
PREFIX  ?= /usr/local
APPDIR  ?= /Applications

OUT ?= /tmp/baybar.png
REGION ?=

.PHONY: all check clean test e2e docs-check docs-generate install uninstall window release
all: $(BIN)/bay $(BIN)/bayd $(APP)

# bayinit runs in front of a service's process inside its container, so it is
# built for Linux, static, and embedded in bay and bayd for both
# architectures (see internal/stack/bayinit.go).
INIT := internal/stack/initbin/bayinit-linux-arm64 internal/stack/initbin/bayinit-linux-amd64

internal/stack/initbin/bayinit-linux-%: cmd/bayinit/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=$* $(GO) build -trimpath -ldflags "-s -w" -o $@ ./cmd/bayinit

$(BIN)/bay: $(INIT) $(shell find cmd/bay internal \( -name '*.go' -o -name '*.c' -o -name '*.h' \))
	$(GO) build -o $@ ./cmd/bay

$(BIN)/bayd: $(INIT) $(shell find cmd/bayd internal \( -name '*.go' -o -name '*.c' -o -name '*.h' \))
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

test: $(INIT)
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
	@# A running dashboard keeps the old build in memory: it is quit before the
	@# app is replaced and opened again after.
	@pgrep -x baybar >/dev/null && touch $(BIN)/.baybar-running || rm -f $(BIN)/.baybar-running
	-@pkill -x baybar; while pgrep -x baybar >/dev/null; do sleep 0.1; done
	rm -rf "$(APPDIR)/baybar.app"
	cp -R $(APP) "$(APPDIR)/"
	@if [ -f $(BIN)/.baybar-running ] && [ "$$(id -u)" != 0 ]; then \
	  rm -f $(BIN)/.baybar-running; open "$(APPDIR)/baybar.app"; echo "reopened baybar"; fi
	@# The agent skill and the MCP registration follow the installed binary. As
	@# root they would land in root's home, so sudo leaves them to "bay install".
	@# A machine without an agent has neither, which is not a failure here.
	@if [ "$$(id -u)" != 0 ]; then \
	  "$(PREFIX)/bin/bay" skill install || true; \
	  "$(PREFIX)/bin/bay" mcp install || true; \
	fi
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

# release builds bay and bayd for each platform into dist/, one archive per
# platform, and SHA256SUMS over the archives. VERSION is the tag, which the
# binaries print with "bay version". The macOS builds use cgo, so a release is
# built on macOS; the Linux builds need no cgo.
#   make release VERSION=v0.1.0
DIST ?= dist
release: $(INIT)
	@test -n "$(VERSION)" || { echo "make release VERSION=vX.Y.Z"; exit 1; }
	rm -rf $(DIST) && mkdir -p $(DIST)
	@set -e; for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do \
	  os=$${target%/*}; arch=$${target#*/}; name=bay_$(VERSION)_$${os}_$${arch}; \
	  mkdir -p $(DIST)/$$name; \
	  if [ $$os = darwin ]; then cgo=1; cc="clang -arch $$( [ $$arch = amd64 ] && echo x86_64 || echo arm64 )"; else cgo=0; cc=cc; fi; \
	  for cmd in bay bayd; do \
	    CGO_ENABLED=$$cgo CC="$$cc" GOOS=$$os GOARCH=$$arch $(GO) build -trimpath \
	      -ldflags "-s -w -X main.buildVersion=$(VERSION)" -o $(DIST)/$$name/$$cmd ./cmd/$$cmd; \
	  done; \
	  cp LICENSE* README.md $(DIST)/$$name/ 2>/dev/null || true; \
	  tar -C $(DIST) -czf $(DIST)/$$name.tar.gz $$name; rm -rf $(DIST)/$$name; \
	done
	cd $(DIST) && shasum -a 256 *.tar.gz > SHA256SUMS
	@cat $(DIST)/SHA256SUMS
