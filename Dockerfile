FROM alpine:3.21

ARG TARGETARCH

COPY dist/linux-${TARGETARCH}/grokforge-linux-${TARGETARCH} /usr/local/bin/grokforge
COPY config.defaults.toml /app/config.toml

RUN apk add --no-cache ca-certificates tzdata && \
    chmod +x /usr/local/bin/grokforge && \
    adduser -D -u 1000 grokforge && \
    mkdir -p /app/data && \
    chown -R grokforge:grokforge /app

USER grokforge

WORKDIR /app
VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["grokforge"]
CMD ["-config", "/app/config.toml"]
