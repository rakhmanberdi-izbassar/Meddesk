# syntax=docker/dockerfile:1
FROM golang:1.27.2-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=0.5.1
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bootstrap-admin ./cmd/bootstrap-admin

FROM alpine:3.23
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 meddesk \
    && adduser -S -D -H -u 10001 -G meddesk meddesk \
    && mkdir -p /var/lib/meddesk/media \
    && chown -R meddesk:meddesk /var/lib/meddesk
WORKDIR /app
COPY --from=builder --chown=meddesk:meddesk /out/api /app/api
COPY --from=builder --chown=meddesk:meddesk /out/seed /app/seed
COPY --from=builder --chown=meddesk:meddesk /out/bootstrap-admin /app/bootstrap-admin
COPY --chown=meddesk:meddesk scripts/setup-admin.sh /app/setup-admin.sh
USER meddesk:meddesk
ENV HTTP_ADDR=:8081 MEDIA_DIR=/var/lib/meddesk/media
EXPOSE 8081
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD ["/app/api", "-healthcheck"]
ENTRYPOINT ["/app/api"]
