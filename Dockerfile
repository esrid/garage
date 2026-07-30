FROM golang:1.27rc2-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o app ./cmd

FROM alpine:3.22

RUN addgroup -S app && adduser -S -G app app
COPY --from=builder --chown=app:app /build/app /app

ENV HTTP_ADDR=:8080
EXPOSE 8080
USER app

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD ["wget", "--spider", "-q", "http://127.0.0.1:8080/healthz"]

ENTRYPOINT ["/app"]
