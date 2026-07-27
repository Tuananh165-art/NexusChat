# Go parameters
GOCMD=go
GOTEST=$(GOCMD) test
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOINSTALL=$(GOCMD) install

SVCS=chat match uploader user
REALTIME_SVCS=presence notification call

VERSION=v0.0.0

.PHONY: proto doc

all: build test
test:
	$(GOTEST) -gcflags=-l -v -cover -coverpkg=./... -coverprofile=cover.out ./...
build: dep doc
	$(GOBUILD) -ldflags="-X github.com/Tuananh165-art/NexusChat/cmd.Version=$(VERSION) -w -s" -o server ./main.go

dep: wire
	$(shell $(GOCMD) env GOPATH)/bin/wire ./internal/wire
proto:
	protoc proto/*/*.proto --go_out=plugins=grpc:.
doc: swag
	for svc in $(SVCS); do \
		$(shell $(GOCMD) env GOPATH)/bin/swag init -g http.go -d pkg/$$svc -o docs/$$svc --instanceName $$svc --parseDependency --parseInternal; \
	done

wire:
	GO111MODULE=on $(GOINSTALL) github.com/google/wire/cmd/wire@v0.7.0
swag:
	GO111MODULE=on $(GOINSTALL) github.com/swaggo/swag/cmd/swag@v1.16.6

docker: docker-api docker-web
docker-api:
	@docker build -f ./build/Dockerfile.api --build-arg VERSION=$(VERSION) -t tuananh165/nexuschat-api:kafka .
docker-web:
	@docker build -f ./build/Dockerfile.web --build-arg VERSION=$(VERSION) -t tuananh165/nexuschat-web:kafka .
docker-realtime:
	@docker build -f ./build/Dockerfile.realtime --build-arg VERSION=$(VERSION) --build-arg SERVICE=presence -t tuananh165/nexuschat-presence:$(VERSION) .
	@docker build -f ./build/Dockerfile.realtime --build-arg VERSION=$(VERSION) --build-arg SERVICE=notification -t tuananh165/nexuschat-notification:$(VERSION) .
	@docker build -f ./build/Dockerfile.realtime --build-arg VERSION=$(VERSION) --build-arg SERVICE=call -t tuananh165/nexuschat-call:$(VERSION) .
clean:
	$(GOCLEAN)
	rm -f server
