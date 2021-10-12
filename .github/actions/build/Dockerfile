FROM golang:1.16-buster

RUN \
    apt-get update && \
    apt-get install -y ca-certificates openssl zip curl jq gcc-multilib g++-multilib gcc-aarch64-linux-gnu && \
    update-ca-certificates && \
    rm -rf /var/lib/apt

COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]