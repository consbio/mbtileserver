FROM golang:1.23-bookworm

RUN \
    apt-get update && \
    apt-get install -y ca-certificates openssl zip curl jq gcc-multilib \
    g++-multilib && \
    update-ca-certificates && \
    rm -rf /var/lib/apt

COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]