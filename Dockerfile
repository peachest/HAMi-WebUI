FROM node:21.6.2 AS builder

WORKDIR /src

RUN npm install -g pnpm

COPY . .

RUN make build-all

FROM node:21.6.2-slim

ARG BUILD_VERSION
ARG BUILD_TIME
ARG CI_COMMIT_SHA
LABEL build_name="hami-webui-fe" build_version="${BUILD_VERSION}" build_time="${BUILD_TIME}" commit_id="${CI_COMMIT_SHA}"

COPY --from=builder /src/dist/ /apps/dist/
COPY --from=builder /src/node_modules/ /apps/node_modules/
COPY --from=builder /src/public/ /apps/public/
