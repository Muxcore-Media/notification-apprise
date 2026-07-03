FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY notification-apprise/ /build/notification-apprise/
WORKDIR /build/notification-apprise
RUN go mod download && CGO_ENABLED=0 go build -o /notification-apprise ./cmd/module
FROM alpine:3.21
RUN adduser -D -h /data app
USER app
WORKDIR /app
COPY --from=builder /notification-apprise .
ENTRYPOINT ["./notification-apprise"]
