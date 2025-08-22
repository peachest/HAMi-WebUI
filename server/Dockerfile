FROM --platform=${BUILDPLATFORM} harbor.transwarp.io/aip/base/golang-builder:1.24-ubuntu24.04 AS builder
ARG TARGETARCH
ARG ARCH=${TARGETARCH}

ENV GOSUMDB=off
ENV GOPROXY=http://172.16.251.11:1111,https://goproxy.io/,https://mirrors.aliyun.com/goproxy,direct
ENV GOBIN=/opt/go/bin
ENV https_proxy=http://172.16.80.252:3128 http_proxy=http://172.16.80.252:3128 all_proxy=socks5://172.16.80.252:3128 no_proxy="127.0.0.1,172.16.0.0/16,172.17.120.0/24"

# RUN sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list.d/debian.sources

WORKDIR /src

COPY . .

RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler


RUN PATH=/root/go/bin:$PATH make build

FROM harbor.transwarp.io/aip/base/alpine-runner:3.19
ARG TARGETARCH

ARG ARCH=${TARGETARCH}

COPY --from=builder /src/build/ /apps/
