.PHONY: build run test clean docker-build docker-push deploy

APP_NAME := familybot
REGISTRY := docker.tazhate.com
IMAGE := $(REGISTRY)/$(APP_NAME)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	CGO_ENABLED=1 go build -o bin/$(APP_NAME) ./cmd/bot

run:
	go run ./cmd/bot

test:
	go test -v ./...

clean:
	rm -rf bin/

docker-build:
	docker build -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-push:
	docker push $(IMAGE):$(VERSION)
	docker push $(IMAGE):latest

deploy:
	kubectl apply -f k8s/
	kubectl -n familybot rollout restart deploy/familybot

logs:
	kubectl -n familybot logs -f deploy/familybot

status:
	kubectl -n familybot get pods

tidy:
	go mod tidy

lint:
	golangci-lint run ./...
