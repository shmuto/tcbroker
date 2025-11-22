# Makefile for tcbroker

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOTOOL=$(GOCMD) tool

# Binary Name
BINARY_NAME=tcbroker
BINARY_UNIX=$(BINARY_NAME)

# Docker parameters
DOCKER_COMPOSE=docker compose -f tests/compose.yaml
DOCKER_EXEC=docker exec tcbroker-broker
DOCKER_EXEC_IT=docker exec -it tcbroker-broker

all: build

build:
	CGO_ENABLED=0 $(GOBUILD) -buildvcs=false -o $(BINARY_NAME) ./cmd/tcbroker

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# Docker commands
docker-build:
	$(DOCKER_COMPOSE) build

docker-up:
	$(DOCKER_COMPOSE) up -d
	@echo ""
	@echo "Containers started! Use 'make docker-shell' to enter the container."

docker-down:
	$(DOCKER_COMPOSE) down

docker-restart: docker-down docker-up

docker-rebuild:
	$(DOCKER_COMPOSE) down
	$(DOCKER_COMPOSE) build --no-cache
	$(DOCKER_COMPOSE) up -d
	@echo ""
	@echo "Containers rebuilt! Use 'make docker-shell' to enter the container."

docker-shell:
	$(DOCKER_EXEC_IT) bash

docker-logs:
	$(DOCKER_COMPOSE) logs -f

docker-test:
	@echo "Running full automated test (requires sudo)..."
	@sudo ./tests/setup.sh test

docker-test-interactive:
	@echo "Starting interactive test environment (requires sudo)..."
	@sudo ./tests/setup.sh interactive

docker-ps:
	$(DOCKER_COMPOSE) ps

# Setup veth pairs only (requires containers to be running)
docker-setup-veth:
	@echo "Setting up veth pairs (requires sudo)..."
	@sudo ./tests/setup.sh setup-veth

# Cleanup veth pairs only
docker-cleanup-veth:
	@echo "Cleaning up veth pairs (requires sudo)..."
	@sudo ./tests/setup.sh cleanup-veth

# Cleanup Docker environment
docker-clean:
	@echo "Cleaning up Docker environment (requires sudo)..."
	@sudo ./tests/setup.sh down

.PHONY: all build test clean \
	docker-build docker-up docker-down docker-restart docker-rebuild \
	docker-shell docker-logs docker-test docker-test-interactive docker-ps \
	docker-setup-veth docker-cleanup-veth docker-clean
