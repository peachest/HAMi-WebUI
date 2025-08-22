PATH := $(HOME)/go/bin:$(PATH)
BUILD_DATE := $(shell date "+%Y%m%d")
BUILD_TIME := $(shell date "+%Y%m%dT%H%M%S")
CI_COMMIT_SHA ?= 00000000
CI_COMMIT_TAG ?= 0.0.0
COMMIT_ID := $(shell echo ${CI_COMMIT_SHA} | cut -c-8)
DOCKER_REPO_URL ?= harbor.transwarp.io


DOCKER_IMAGE ?= ${DOCKER_REPO_URL}/aip/projecthami/hami-webui-be-oss
CMD_PATH ?= ./cmd/
BUILD_PATH ?= ./build/
VERSION ?= $(shell git describe --tags --always)
DIRS = $(shell ls $(CMD_PATH))

REF=$(if $(CI_COMMIT_REF_NAME),$(CI_COMMIT_REF_NAME),$(shell git rev-parse --abbrev-ref HEAD))

# 1. Install Go dependencies
.PHONY: install-deps
install-deps:
	go install github.com/google/wire/cmd/wire@latest
	go install github.com/envoyproxy/protoc-gen-validate@latest
	go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest

# 2. Generate proto and wire files
.PHONY: generate
generate: generate-proto generate-wire

.PHONY: generate-proto
generate-proto:
	kratos proto client .

.PHONY: generate-wire
generate-wire: tidy-mod
	@ for dir in $(DIRS); \
	do \
		cd ${CMD_PATH}$$dir/ && wire . && cd ../../; \
	done

# 3. Clean Go modules
.PHONY: tidy-mod
tidy-mod:
	 go mod tidy

# 4. Build-related commands
.PHONY: build-linux
build-linux:
	@ for dir in $(DIRS); \
	do \
		GO_ENABLED=1 GOOS=linux GOARCH=${ARCH} go build -o $(BUILD_PATH)$$dir ${CMD_PATH}$$dir; \
		echo "Built $$dir for Linux"; \
	done

.PHONY: build-local
build-local:
	@ for dir in $(DIRS); \
	do \
		go build -o $(BUILD_PATH)$$dir ${CMD_PATH}$$dir; \
		echo "Built $$dir"; \
	done

# 5. Docker image commands
.PHONY: build-image
build-image:
	docker build --platform linux/amd64 -t ${DOCKER_IMAGE}:${VERSION} .

.PHONY: buildx-image
buildx-image:
	echo ${REF}
	docker buildx build --platform linux/amd64,linux/arm64 --push -t ${DOCKER_IMAGE}:${REF} .
.PHONY: push-image
push-image:
	docker push ${DOCKER_IMAGE}:${VERSION}

.PHONY: save-image
save-image:
	docker save -o $(BUILD_PATH)hami_webui_release_${VERSION}.tar ${DOCKER_IMAGE}:${VERSION}
	gzip -f $(BUILD_PATH)hami_webui_release_${VERSION}.tar

# 6. Debugging and Testing
.PHONY: run
run: install-deps generate run-debug

.PHONY: run-debug
run-debug:
	kratos run

.PHONY: run-tests
run-tests:
	go test .

.PHONY: run-vet
run-vet:
	go vet ./...

# 7. Cleaning
.PHONY: clean-build
clean-build:
	rm -rf $(BUILD_PATH)

# 8. Complete build workflow
.PHONY: build
build: install-deps generate build-linux

# 9. Release workflow
.PHONY: release
release: build-image push-image