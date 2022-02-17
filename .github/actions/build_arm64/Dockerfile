FROM golang:1.17-bullseye

RUN \
    dpkg --add-architecture arm64 && \
    apt-get update && \
    apt-get install -y ca-certificates openssl zip curl jq \
    gcc-10-aarch64-linux-gnu gcc-aarch64-linux-gnu libsqlite3-dev:arm64 && \
    update-ca-certificates && \
    rm -rf /var/lib/apt

COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]