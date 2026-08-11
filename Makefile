# XDL build orchestration.
#
#   make core      — build the Go backend (universal binary) into build/xdl-core
#   make generate  — regenerate XDL.xcodeproj from project.yml
#   make dev       — core + generate + unsigned Debug build
#   make run       — dev + launch the app
#   make clean     — remove build products

APP_NAME := XDL
BUILD_DIR := build
CORE_BIN := $(BUILD_DIR)/xdl-core
DERIVED := $(BUILD_DIR)/DerivedData
APP := $(DERIVED)/Build/Products/Debug/$(APP_NAME).app
# Native UN notifications need a valid signing identity. Set SIGN_ID to your
# "Developer ID Application: …" identity and build with `make dev SIGNED=1`.
# Unsigned (ad-hoc) dev builds fall back to osascript notifications.
SIGN_ID ?=

.PHONY: core generate dev run clean

core:
	mkdir -p $(BUILD_DIR)
	cd core && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o ../$(BUILD_DIR)/xdl-core-arm64 ./cmd/xdl-core
	cd core && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ../$(BUILD_DIR)/xdl-core-amd64 ./cmd/xdl-core
	lipo -create -output $(CORE_BIN) $(BUILD_DIR)/xdl-core-arm64 $(BUILD_DIR)/xdl-core-amd64
	rm -f $(BUILD_DIR)/xdl-core-arm64 $(BUILD_DIR)/xdl-core-amd64
	codesign -f -s - $(CORE_BIN)
	@echo "core: $$(du -h $(CORE_BIN) | cut -f1) universal binary"

generate:
	xcodegen generate

dev: core generate
ifdef SIGNED
	xcodebuild -project $(APP_NAME).xcodeproj -scheme $(APP_NAME) -configuration Debug \
		-derivedDataPath $(DERIVED) \
		CODE_SIGN_STYLE=Manual CODE_SIGN_IDENTITY="$(SIGN_ID)" build
else
	xcodebuild -project $(APP_NAME).xcodeproj -scheme $(APP_NAME) -configuration Debug \
		-derivedDataPath $(DERIVED) CODE_SIGNING_ALLOWED=NO build
endif

run: dev
	open $(APP)

clean:
	rm -rf $(BUILD_DIR) $(APP_NAME).xcodeproj
