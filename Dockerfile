# Base
FROM golang:1.21.1-alpine AS builder
RUN apk add --no-cache build-base
WORKDIR /app
COPY . /app
RUN go mod download
RUN go build ./cmd/cloudlist

# Release
FROM alpine:3.18.4
RUN apk -U upgrade --no-cache \
    && apk add --no-cache bind-tools ca-certificates
COPY --from=builder /app/cloudlist /usr/local/bin/

ENTRYPOINT ["cloudlist"]