BINARY := pr-attention
CMD := ./cmd/pr-attention
LOCAL_BIN ?= $(HOME)/.local/bin

.PHONY: build test

build:
	@mkdir -p "$(LOCAL_BIN)"
	go build -o "$(LOCAL_BIN)/$(BINARY)" "$(CMD)"
	@echo "Installed $(BINARY) to $(LOCAL_BIN)/$(BINARY)"

test:
	go test ./...
