CGO_ENABLED ?= 0

DOCKER_REPO ?= sagaxyz
IMAGE_NAME = cosmos-exporter

VERSION = $(shell git describe --tags --dirty --always --abbrev=4 | sed -e 's/^v//')
BUILD_ARGS = -ldflags '-X "main.Version=$(VERSION)"'

all: build

.PHONY: build
build:
	CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_ARGS) -o build/cosmos-exporter

.PHONY: start
start:
	build/cosmos-exporter

.PHONY: run
run: build start

.PHONY: clean
clean:
	rm -rf build/*

.PHONY: docker-build
docker-build:
	docker build -t ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION} -t ${DOCKER_REPO}/${IMAGE_NAME}:latest -f deploy/Dockerfile .

.PHONY: docker-push
docker-push:
	docker push ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION}
	docker push ${DOCKER_REPO}/${IMAGE_NAME}:latest