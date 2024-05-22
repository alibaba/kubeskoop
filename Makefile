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
	CGO_ENABLED=0 go build -o bin/skoop -ldflags $(ldflags) ./cmd/skoop

.PHONY: build-collector
build-collector:
	CGO_ENABLED=0 go build -o bin/pod-collector -ldflags $(ldflags) ./cmd/collector

.PHONY: build-controller
build-controller:
	go build -o bin/controller -ldflags $(ldflags) ./cmd/controller

.PHONY: build-btfhack
build-btfhack:
	CGO_ENABLED=0 go build -o bin/btfhack -ldflags $(ldflags) ./cmd/btfhack

.PHONY: build-webconsole
build-webconsole:
	cd webui && CGO_ENABLED=0 go build -o ../bin/webconsole -ldflags $(ldflags) .

.PHONY: generate-bpf
generate-bpf:
	go generate ./pkg/exporter/probe/...

.PHONY: generate-bpf-in-container
generate-bpf-in-container:
	docker run --rm -v $(PWD):/go/src/github.com/alibaba/kubeskoop --workdir /go/src/github.com/alibaba/kubeskoop kubeskoop/bpf-build:go121-clang17 make generate-bpf
