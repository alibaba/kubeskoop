SKOOP_REPO ?= kubeskoop/kubeskoop
TAG ?= latest

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

fmt:
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

build: ## build kubeskoop image
	docker build -t $(SKOOP_REPO):$(TAG) .

push: build ## push kubeskoop image
	docker push $(SKOOP_REPO):$(TAG)
