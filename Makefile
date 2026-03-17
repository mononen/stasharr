REGISTRY := adoah
API_IMAGE := $(REGISTRY)/stasharr-api
UI_IMAGE  := $(REGISTRY)/stasharr-ui
PLATFORM  := linux/amd64

.PHONY: up

up:
	docker compose -f docker-compose.dev.yml up --build 2>&1

build: 
	docker compose -f docker-compose.dev.yml build

upd: 
	docker compose -f docker-compose.dev.yml up -d  

prodbuild: build-api build-ui

push: prodbuild hubpush

build-api:
	docker buildx build \
		--platform $(PLATFORM) \
		--file docker/api.Dockerfile \
		--target production \
		--tag $(API_IMAGE):latest \
		--load \
		.

build-ui:
	docker buildx build \
		--platform $(PLATFORM) \
		--file docker/ui.Dockerfile \
		--target production \
		--tag $(UI_IMAGE):latest \
		--load \
		.

hubpush:
	docker push ${UI_IMAGE}:latest && docker push ${API_IMAGE}:latest