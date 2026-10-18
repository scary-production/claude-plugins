GOLANGCI_LINT_VERSION := v2.14.0
# Claude Code для проверки плагинов: validate и test модов — API модов меняется между версиями.
CLAUDE_CODE_VERSION   := 2.1.302
IMAGE                 ?= ghcr.io/scary-production/claude-plugins

.PHONY: all build fmt fmt-check vet lint test shellcheck plugins-check ci image styles

all: fmt vet test build

build:
	go build -trimpath -o bin/claude-plugins ./cmd/claude-plugins

fmt:
	go fmt ./...

fmt-check:
	@files=$$(gofmt -l $$(go list -f '{{.Dir}}' ./...)); test -z "$$files" || { echo "gofmt: $$files"; exit 1; }

vet:
	go vet ./...

lint:
	docker run --rm -v "$(CURDIR)":/src -w /src -v golangci-cache:/root/.cache \
		golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) golangci-lint run ./...

# Скрипты плагинов и statusline.sh против примеров (пакет test) — тоже здесь.
test:
	go test -race -count=1 ./...

shellcheck:
	docker run --rm -v "$(CURDIR)":/mnt -w /mnt koalaman/shellcheck:stable plugins/*/scripts/*.sh

# Маркетплейс и каждый плагин так, как их прочтёт Claude Code; тесты модов.
plugins-check:
	claude plugin validate .
	@for p in plugins/*/; do claude plugin validate "$$p" || exit 1; done
	@for p in plugins/*/; do \
		if ls "$$p"tests/*.test.ts >/dev/null 2>&1; then claude plugin test "$$p" || exit 1; fi; \
	done

ci: fmt-check vet test build plugins-check

image:
	docker buildx build -f docker/Dockerfile -t $(IMAGE):dev --load .

# Стили строки состояния на примере — из собранного образа.
styles: image
	docker run --rm $(IMAGE):dev styles
