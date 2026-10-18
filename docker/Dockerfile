# syntax=docker/dockerfile:1
# Образ установщика: claude-plugins и копия маркетплейса. Каталоги хоста монтируются:
# ~/.claude — в /claude, проект — в /project (README).
ARG GO_VERSION=1.27
ARG ALPINE=3.24

# --- Builder: на платформе сборки, кросс-компиляцией под целевую ---
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine${ALPINE} AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false -ldflags="-s" -o /out/claude-plugins ./cmd/claude-plugins

# --- Runtime ---
FROM alpine:${ALPINE}

LABEL org.opencontainers.image.source="https://github.com/scary-production/claude-plugins" \
      org.opencontainers.image.description="Установщик плагинов Claude Code из маркетплейса scary-production" \
      org.opencontainers.image.licenses="MIT"

# bash и jq — для claude-plugins styles: стили рисует тот же statusline.sh, что и на хосте.
RUN apk add --no-cache bash jq

COPY --link .claude-plugin /opt/marketplace/.claude-plugin
COPY --link plugins /opt/marketplace/plugins
COPY --link --from=builder /out/claude-plugins /usr/local/bin/claude-plugins

ENTRYPOINT ["claude-plugins"]
CMD ["help"]
