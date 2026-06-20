# Build context is the repo root (see docker/docker-compose.yml), so this
# Dockerfile can COPY the full module.

FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/banking ./cmd/api

FROM alpine:3.20

# ca-certificates is required so the mTLS HTTP client can verify C6's
# server certificate chain over TLS.
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 banking

COPY --from=builder /out/banking /usr/local/bin/banking

USER banking
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/banking"]
