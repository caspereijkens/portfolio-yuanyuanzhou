# Variables
IMAGE_NAME = salmonsalmon/portfolio-yuanyuan
VERSION ?= v0.1.6  # Default version if not provided
IMAGE_TAG = $(IMAGE_NAME):$(VERSION)
LATEST_TAG = $(IMAGE_NAME):latest
PROXY_COMPOSE_FILE = nginx-proxy-compose.yaml
APP_COMPOSE_FILE = go-app-compose.yaml

# Commands
.PHONY: all build push clean deploy undeploy help

# Default command to show help
all: help

# Build the Docker image with a specific version tag
build:
	@echo "Building Docker image with tag $(IMAGE_TAG)"
	docker build -t $(IMAGE_TAG) .

# Tag the image as 'latest'
tag-latest:
	@echo "Tagging $(IMAGE_TAG) as $(LATEST_TAG)"
	docker tag $(IMAGE_TAG) $(LATEST_TAG)

# Push the image to Docker Hub
push: build tag-latest
	@echo "Pushing Docker image $(IMAGE_TAG) to Docker Hub"
	docker push $(IMAGE_TAG)
	@echo "Pushing Docker image $(LATEST_TAG) to Docker Hub"
	docker push $(LATEST_TAG)

# Remove local Docker images
clean:
	@echo "Removing local Docker images: $(IMAGE_TAG) and $(LATEST_TAG)"
	docker rmi $(IMAGE_TAG) $(LATEST_TAG) || true

# Deploy the application using docker-compose
deploy:
	@echo "Deploying the application with docker-compose"
	docker-compose -f $(PROXY_COMPOSE_FILE) up -d 
	docker-compose -f $(APP_COMPOSE_FILE) up -d 

# Take down the application using docker-compose
undeploy:
	@echo "Taking down the application with docker-compose"
	docker-compose -f $(APP_COMPOSE_FILE) down
	docker-compose -f $(PROXY_COMPOSE_FILE) down

deploy-app:
	@echo "Deploying the application with docker-compose"
	docker-compose -f $(APP_COMPOSE_FILE) up -d 

# Take down the application using docker-compose
undeploy-app:
	@echo "Taking down the application with docker-compose"
	docker-compose -f $(APP_COMPOSE_FILE) down

reset-app: undeploy-app clean deploy-app

exec:
	@docker exec -it $$(docker ps -q -f "ancestor=$(IMAGE_TAG)") sh

build-ops-bins:
	@go build -o bin/cleanup-filepaths  ops/cleanup-filepaths/main.go
	@go build -o bin/make-thumbnails  ops/make-thumbnails/main.go

# Show help message
help:
	@echo "Makefile Commands:"
	@echo "  build        - Build the Docker image with the specified version tag"
	@echo "  push         - Build, tag as 'latest', and push the image to Docker Hub"
	@echo "  clean        - Remove the local Docker images"
	@echo "  deploy       - Deploy the application using docker-compose"
	@echo "  undeploy     - Take down the application using docker-compose"
	@echo "  help         - Show this help message"
	@echo ""
	@echo "You can set the VERSION variable when calling make:"
	@echo "  make build VERSION=v1.2.3"
	@echo "  make push VERSION=v1.2.3"
