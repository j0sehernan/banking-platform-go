# Multi-stage build: imagen final liviana basada en alpine.
# Pasamos el go.work + pkg + service para que el go workspace funcione
# dentro del build context (asumimos build desde la raíz del repo).

FROM golang:1.23-alpine AS builder

WORKDIR /build

# Copio go.work y los modulos para resolver deps con cache de capas
COPY go.work ./
COPY pkg/ ./pkg/
COPY services/accounts-ms/ ./services/accounts-ms/

# Necesito vaciar las refs a otros services del go.work para que no falle
# (no las necesitamos para compilar accounts-ms)
RUN cat > go.work <<EOF
go 1.23
use (
    ./pkg
    ./services/accounts-ms
)
EOF

WORKDIR /build/services/accounts-ms
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/accounts-ms ./cmd

# === runtime ===
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/accounts-ms /app/accounts-ms
EXPOSE 8080
ENTRYPOINT ["/app/accounts-ms"]
