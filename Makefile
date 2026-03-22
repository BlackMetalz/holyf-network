APP_NAME := holyf-network
BIN_DIR := bin
BIN_PATH := $(BIN_DIR)/$(APP_NAME)
ARGS ?=

.PHONY: help build local clean test

help:
	@echo "Targets:"
	@echo "  make build               Build binary to $(BIN_PATH)"
	@echo "  make local ARGS=\"...\"    Build then run with sudo"
	@echo "  make test                Run go test ./..."
	@echo "  make clean               Remove built binaries"

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_PATH) .

local: build
	sudo -E ./$(BIN_PATH) $(ARGS)

test:
	go test ./...

clean:
	rm -rf $(BIN_DIR)
