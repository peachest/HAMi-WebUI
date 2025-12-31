FROM --platform=${BUILDPLATFORM} harbor.transwarp.io/aip/base/golang-builder:1.24-ubuntu24.04 AS builder
ARG TARGETARCH
ARG ARCH=${TARGETARCH}

# ENV GOSUMDB=off
ENV GOPROXY=http://172.16.251.11:1111,https://goproxy.cn,https://mirrors.aliyun.com/goproxy,https://goproxy.io/,direct
ENV GOBIN=/opt/go/bin

# RUN sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list.d/debian.sources

WORKDIR /src

COPY . .
RUN sed -i 's|http://mirrors.aliyun.com/ubuntu/|https://mirrors.ustc.edu.cn/ubuntu/|g'  /etc/apt/sources.list.d/ubuntu.sources && \
    apt-get update && \
    apt-get install -y --no-install-recommends protobuf-compiler && \
    rm -rf /var/lib/apt/lists/*

# RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler


RUN PATH=/root/go/bin:$PATH make build

FROM harbor.transwarp.io/aip/base/alpine-runner:3.19
ARG TARGETARCH

ARG ARCH=${TARGETARCH}

COPY --from=builder /src/build/ /apps/
