# Multi-stage build: Go builder + minimal alpine runtime.
# Builds binder-mcp for the host platform.
#
#   docker build -t binder-mcp .
#   docker run --rm binder-mcp --mode remote

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/binder-mcp ./cmd/binder-mcp/

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/binder-mcp /usr/local/bin/binder-mcp

EXPOSE 7100

LABEL org.opencontainers.image.source="https://github.com/AndroidGoLab/binder"
LABEL org.opencontainers.image.description="Android device automation via Binder IPC — MCP server for AI agents"

ENTRYPOINT ["binder-mcp"]
