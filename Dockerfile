# Stage 1: compile mbtileserver
FROM golang:1.16-alpine3.12

WORKDIR /
RUN apk add git build-base
COPY . .

RUN GOOS=linux go build -o /mbtileserver


# Stage 2: start from a smaller image
FROM alpine:3.12

WORKDIR /

# Link libs to get around issues using musl
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

# Install AWS CLI
RUN apk add --no-cache \
        bash \
        python3 \
        py3-pip \
    && pip3 install --upgrade pip \
    && pip3 install \
        awscli \
    && rm -rf /var/cache/apk/*

# copy the executable to the empty container
COPY --from=0 /mbtileserver /mbtileserver

# run entrypoint script rather than mbtileserver
COPY ./docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]