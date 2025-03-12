FROM --platform=${BUILDPLATFORM} 172.16.1.99/aip/base/golang-builder:1.23-ubuntu24.04 AS builder
ARG TARGETARCH
ARG ARCH=${TARGETARCH}

ENV GOPROXY=http://172.16.251.11:1111,https://goproxy.io/,https://mirrors.aliyun.com/goproxy,direct

# RUN sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list.d/debian.sources

WORKDIR /src

COPY . .

RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler

RUN make build

FROM 172.16.1.99/aip/base/alpine-runner:3.19
ARG TARGETARCH

ARG ARCH=${TARGETARCH}

COPY --from=builder /src/build/ /apps/
