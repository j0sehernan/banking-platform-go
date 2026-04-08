FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.work ./
COPY pkg/ ./pkg/
COPY services/transactions-ms/ ./services/transactions-ms/

RUN cat > go.work <<EOF
go 1.23
use (
    ./pkg
    ./services/transactions-ms
)
EOF

WORKDIR /build/services/transactions-ms
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/transactions-ms ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/transactions-ms /app/transactions-ms
EXPOSE 8080
ENTRYPOINT ["/app/transactions-ms"]
