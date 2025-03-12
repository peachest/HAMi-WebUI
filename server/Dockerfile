FROM golang:1.23.1 AS builder

ENV GOPROXY=http://172.16.251.11:1111,https://goproxy.cn/,https://goproxy.io/,https://mirrors.aliyun.com/goproxy,direct

RUN sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list.d/debian.sources

WORKDIR /src

COPY . .

RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler

RUN make build

FROM debian:stable-slim

COPY --from=builder /src/build/ /apps/
