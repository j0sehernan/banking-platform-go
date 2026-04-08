FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.work ./
COPY pkg/ ./pkg/
COPY services/llm-ms/ ./services/llm-ms/

RUN cat > go.work <<EOF
go 1.23.0

toolchain go1.23.1

use (
    ./pkg
    ./services/llm-ms
)
EOF

WORKDIR /build/services/llm-ms
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/llm-ms ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/llm-ms /app/llm-ms
EXPOSE 8080
ENTRYPOINT ["/app/llm-ms"]
