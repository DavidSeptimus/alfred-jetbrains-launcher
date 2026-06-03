VERSION    := $(shell cat VERSION 2>/dev/null || echo 0.1.0)
# CHANNEL gates self-update: "release" builds (made by `make dist`) may auto-update;
# the default "dev" (make build/install) refuses to, so source builds aren't clobbered.
CHANNEL    ?= dev
BUNDLEID   := com.davidseptimus.jetbrains-launcher
BUNDLE     := build/jb-bundle
ALFRED_DIR := $(HOME)/Library/Application Support/Alfred/Alfred.alfredpreferences/workflows
# Where the installed workflow keeps its durable data (pins, last search, and the
# update-check cache) — Alfred sets alfred_workflow_data to this at runtime.
ALFRED_DATA := $(HOME)/Library/Application Support/Alfred/Workflow Data/$(BUNDLEID)
# The data dir the binary falls back to when run outside Alfred (e.g. a shell).
DEFAULT_DATA := $(HOME)/Library/Application Support/jb-alfred
# Recursive (=) so a target-specific CHANNEL (e.g. on dist) is picked up.
LDFLAGS     = -s -w -X main.version=$(VERSION) -X main.channel=$(CHANNEL)
GOBUILD     = CGO_ENABLED=0 GOOS=darwin go build -ldflags "$(LDFLAGS)"

.PHONY: all build build-universal plist icons bundle install dist test fmt vet clean wipe-update-cache

all: bundle

## build: compile the arm64 binary into the bundle (fast local dev)
build:
	mkdir -p $(BUNDLE)
	GOARCH=arm64 $(GOBUILD) -o $(BUNDLE)/jb ./cmd/jb

## build-universal: compile a fat arm64+amd64 binary (for distribution)
build-universal:
	mkdir -p $(BUNDLE) build/tmp
	GOARCH=arm64 $(GOBUILD) -o build/tmp/jb-arm64 ./cmd/jb
	GOARCH=amd64 $(GOBUILD) -o build/tmp/jb-amd64 ./cmd/jb
	lipo -create -output $(BUNDLE)/jb build/tmp/jb-arm64 build/tmp/jb-amd64

## plist: regenerate info.plist from workflow/ides.json + per-object canvas icons
## (run after `icons` so the per-object <uid>.png files can be populated)
plist:
	mkdir -p $(BUNDLE)
	go run ./cmd/genplist -ides workflow/ides.json -o $(BUNDLE)/info.plist -version $(VERSION) -channel $(CHANNEL) -bundle $(BUNDLE)

## icons: stage the vendored fallback icons into the bundle
## (installed IDEs render their own icon at runtime via Alfred's fileicon)
icons:
	mkdir -p $(BUNDLE)
	bash scripts/stage-icons.sh $(BUNDLE)

## bundle: assemble + ad-hoc sign + de-quarantine the workflow bundle
## (icons before plist: plist also copies per-object canvas icons from the bundle)
bundle: build icons plist
	codesign --force -s - $(BUNDLE)/jb
	-/usr/bin/xattr -dr com.apple.quarantine $(BUNDLE)
	@echo "bundle ready at $(BUNDLE)"

## install: symlink the bundle into Alfred's workflows dir (instant-live dev)
install: bundle
	mkdir -p "$(ALFRED_DIR)"
	rm -rf "$(ALFRED_DIR)/$(BUNDLEID)"
	ln -s "$(abspath $(BUNDLE))" "$(ALFRED_DIR)/$(BUNDLEID)"
	@echo "symlinked $(BUNDLE) -> $(ALFRED_DIR)/$(BUNDLEID)"

## dist: package a distributable .alfredworkflow (universal binary, release channel)
## (releases are cut by the GitHub Actions "release" workflow, not a make target)
dist: CHANNEL := release
dist: build-universal icons plist
	codesign --force -s - $(BUNDLE)/jb
	-/usr/bin/xattr -dr com.apple.quarantine $(BUNDLE)
	mkdir -p dist
	rm -f dist/jb-$(VERSION).alfredworkflow
	# Zip the bundle CONTENTS at the archive root (no parent folder, no ._ files)
	# so Alfred finds info.plist at the top level on import.
	ditto -c -k --norsrc --noextattr $(BUNDLE) dist/jb-$(VERSION).alfredworkflow
	@echo "packaged dist/jb-$(VERSION).alfredworkflow"

## test / vet / fmt
test:
	CGO_ENABLED=0 go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

## wipe-update-cache: delete the cached release check so the next `jb` re-checks
## GitHub right away (handy for testing the "update available" banner). Only the
## update cache is removed — pins, forgotten projects, and last search are kept.
wipe-update-cache:
	rm -f "$(ALFRED_DATA)/update-cache.json" "$(DEFAULT_DATA)/update-cache.json"
	@echo "wiped update-cache.json — next 'jb' search will re-check GitHub"

clean:
	rm -rf build dist
