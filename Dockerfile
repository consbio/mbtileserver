FROM golang:1.12.7-stretch

WORKDIR /

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN GOOS=linux GO111MODULE=on go build -o /mbtileserver

FROM alpine:3.10.1
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2
COPY --from=0 /mbtileserver /mbtileserver
WORKDIR /
CMD ["/mbtileserver"]