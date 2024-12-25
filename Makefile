ARTIFACT_NAME = external-dns-utho-webhook

REGISTRY ?= localhost:5001
IMAGE_NAME ?= external-dns-utho-webhook
IMAGE_TAG ?= latest
IMAGE = $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

show: ## Show variables
	@echo "GOPATH: $(GOPATH)"
	@echo "ARTIFACT_NAME: $(ARTIFACT_NAME)"
	@echo "REGISTRY: $(REGISTRY)"
	@echo "IMAGE_NAME: $(IMAGE_NAME)"
	@echo "IMAGE_TAG: $(IMAGE_TAG)"
	@echo "IMAGE: $(IMAGE)"

.PHONY: build-linux
build-linux: ## Build the binary for linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/bin/$(ARTIFACT_NAME) ./cmd/webhook

.PHONY: tidy
tidy: 
	go mod tidy
	go fmt ./...


.PHONY: deploy
deploy: tidy build push

.PHONY: build
build:
	@echo "building external-dns-utho-webhook with version $(VERSION)"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o external-dns-utho-webhook ./cmd/webhook
	@echo "building docker image to dockerhub utho with version $(VERSION)"
	@docker build . -t utho/external-dns-utho-webhook:$(VERSION)

.PHONY: push
push:
	@echo "building docker image to dockerhub utho with version $(VERSION)"
	docker push utho/external-dns-utho-webhook:$(VERSION)

.PHONY: build-local
build-local: ## Build the binary
	CGO_ENABLED=0 go build -o $(ARTIFACT_NAME) ./cmd/webhook

.PHONY: run
run:build-local ## Run the binary on local machine
	build/bin/external-dns-utho-webhook
