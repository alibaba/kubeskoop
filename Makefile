# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

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
ldflags="-X $(VERSION_PKG).Version=$(TAG) -X $(VERSION_PKG).Commit=${GIT_COMMIT} -w -s"

.PHONY: all
all: build-exporter build-skoop build-controller build-collector build-btfhack build-webconsole

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

##@ Build

.PHONY: build-exporter
build-exporter: ## Build exporter binary.
	CGO_ENABLED=0 go build -o bin/inspector -ldflags $(ldflags) ./cmd/exporter

.PHONY: build-skoop
build-skoop: ## Build skoop binary.
	CGO_ENABLED=0 go build -o bin/skoop -ldflags $(ldflags) ./cmd/skoop

.PHONY: build-collector
build-collector: ## Build collector binary.
	CGO_ENABLED=0 go build -o bin/pod-collector -ldflags $(ldflags) ./cmd/collector

.PHONY: build-controller
build-controller: ## Build controller binary.
	CGO_ENABLED=0 go build -o bin/controller -ldflags $(ldflags) ./cmd/controller

.PHONY: build-btfhack
build-btfhack: ## Build btfhack binary.
	CGO_ENABLED=0 go build -o bin/btfhack -ldflags $(ldflags) ./cmd/btfhack

.PHONY: build-webconsole
build-webconsole: ## Build webconsole binary.
	cd webui && CGO_ENABLED=0 go build -o ../bin/webconsole -ldflags $(ldflags) .

##@ Dependencies
ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: generate-bpf
generate-bpf: ## Generate bpf.
	go generate ./pkg/exporter/probe/...

.PHONY: generate-bpf-in-container
generate-bpf-in-container: ## Generate bpf in container.
	$(CONTAINER_TOOL) run --rm -v $(PWD):/go/src/github.com/alibaba/kubeskoop --workdir /go/src/github.com/alibaba/kubeskoop kubeskoop/bpf-build:go122-clang17 make generate-bpf
