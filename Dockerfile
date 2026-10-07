FROM alpine:3.23

LABEL org.opencontainers.image.authors="ProjectDiscovery"
LABEL org.opencontainers.image.description="Cloudlist is a multi-cloud tool for getting Assets from Cloud Providers."
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.title="cloudlist"
LABEL org.opencontainers.image.url="https://github.com/projectdiscovery/cloudlist"

RUN apk -U upgrade --no-cache \
    && apk add --no-cache bind-tools ca-certificates

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/cloudlist /usr/local/bin/

ENTRYPOINT ["cloudlist"]
