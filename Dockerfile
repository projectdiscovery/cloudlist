FROM golang:1.20.1-alpine AS builder
RUN apk add --no-cache git
RUN go install -v github.com/projectdiscovery/cloudlist/cmd/cloudlist@latest

FROM alpine:3.17.1
RUN apk -U upgrade --no-cache \
    && apk add --no-cache bind-tools ca-certificates
COPY --from=builder /go/bin/cloudlist /usr/local/bin/

ENTRYPOINT ["cloudlist"]