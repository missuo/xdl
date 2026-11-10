# XDL build orchestration. Everything is Go: the desktop app is built with
# MyGo (pure Go, no cgo), so any machine builds for every platform.
#
#   make app        — XDL.app + .dmg for this Mac in build/darwin-<arch>/
#   make dev        — run the app with live reload
#   make windows    — XDL.exe + installer in build/windows-amd64/
#   make cli        — the `xdl` CLI for this machine in build/xdl
#   make release    — tests, the app, and the CLI for macOS, Linux and Windows
#   make docker     — the headless image (xdl serve)
#   make test       — go vet (every platform) + go test
#   make clean

VERSION ?= $(shell sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' mygo.json)
LDFLAGS := -s -w -X github.com/missuo/xdl/internal/version.Version=$(VERSION)
CLI_TARGETS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: app dev windows cli release docker test clean

app:
	go tool mygo build

dev:
	go tool mygo dev

windows:
	go tool mygo build -platform windows/amd64

cli:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o build/xdl ./cmd/xdl

release: test app
	@for t in $(CLI_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=.exe; \
		out=build/cli/xdl-$(VERSION)-$$os-$$arch$$ext; \
		echo "  $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$out ./cmd/xdl || exit 1; \
	done

docker:
	docker build --build-arg VERSION=$(VERSION) -t xdl:$(VERSION) -t xdl:latest .

test:
	go vet ./...
	GOOS=windows go vet ./...
	GOOS=linux go vet ./cmd/xdl ./internal/cli/... ./backend/...
	go test ./...

clean:
	rm -rf build .mygo
