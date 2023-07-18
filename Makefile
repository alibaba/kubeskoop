SKOOP_REPO ?= kubeskoop/kubeskoop

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
VERSION_PKG=github.com/alibaba/kubeskoop/version
TARGETOS?=linux
TARGETARCH?=amd64

TAG?=${shell git describe --tags --abbrev=7}
GIT_COMMIT=${shell git rev-parse HEAD}
ldflags="-X $(VERSION_PKG).Version=$(TAG) -X $(VERSION_PKG).Commit=${GIT_COMMIT}"

.PHONY: all
all: build-exporter build-skoop build-collector

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: build-exporter
build-exporter:
	CGO_ENABLED=0 go build -o bin/inspector -ldflags $(ldflags) ./cmd/exporter

.PHONY: build-skoop
build-skoop:
	go build -o bin/skoop -ldflags $(ldflags) ./cmd/skoop

.PHONY: build-collector
build-collector:
	GOOS=${TARGETOS} CGO_ENABLED=0 go build -o bin/pod-collector -ldflags $(ldflags) ./cmd/collector

.PHONY: image
image: ## build kubeskoop image
	docker build -t $(SKOOP_REPO):$(TAG) .

.PHONY: push
push: image ## push kubeskoop image
	docker push $(SKOOP_REPO):$(TAG)
