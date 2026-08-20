VERSION?=latest
DOCKER_IMAGE=projecthami/hami-webui-fe
OUT=./dist
PROJECT_NAME?=test-project

BUILD_DATE := $(shell date "+%Y%m%d")
BUILD_TIME := $(shell date "+%Y%m%dT%H%M%S")
CI_COMMIT_SHA ?= 00000000
COMMIT_ID := $(shell echo ${CI_COMMIT_SHA} | cut -c-8)
REF=$(if $(CI_COMMIT_REF_NAME),$(CI_COMMIT_REF_NAME),$(shell git rev-parse --abbrev-ref HEAD))
CLEANED_REF=$(shell echo $(REF) | tr '/' '-')

PACKAGE := hami-webui-fe
BUILD_VERSION := ${CLEANED_REF}-${BUILD_DATE}-${COMMIT_ID}
DOCKER_LABELS := --label build_name=${PACKAGE} --label build_version=${BUILD_VERSION} --label build_time=${BUILD_TIME} --label commit_id=${CI_COMMIT_SHA}

# 按项目最小化构建
ROUTE_FILE=packages/web/src/router/index.js
PROJECT_PATH=packages/web/projects/
DISABLED_PROJECTS?=""

.PHONY: install-modules
install-modules:
	pnpm install

.PHONY: build-all
build-all: install-modules build-bff build-web

.PHONY: build-bff
build-bff:
	pnpm run build

.PHONY: build-web
build-web:
	cd packages/web && pnpm run build

.PHONY: start-dev
start-dev: install-modules start-bff start-web


.PHONY: start-bff
start-bff:
	pnpm run start:dev &

.PHONY: start-web
start-web:
	cd packages/web && pnpm run start:dev

.PHONY: start-prod
start-prod:
	pnpm run start:prod

.PHONY: build-image
build-image:
	docker build --platform linux/amd64 ${DOCKER_LABELS} -t ${DOCKER_IMAGE}:${VERSION} .

.PHONY: push-image
push-image:
	docker push ${DOCKER_IMAGE}:${VERSION}

.PHONY: release
release: build-image push-image