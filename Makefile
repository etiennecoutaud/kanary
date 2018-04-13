HUB :=
REPO := etiennecoutaud
IMAGE := kanary-operator
TAG := dev

build:
	go build -i github.com/etiennecoutaud/kanary/cmd/kanary-operator

run: build
	kubectl apply -f manifests/kanary-crd.yml
	./kanary-operator -kubeconfig=$(HOME)/.kube/config -v=2 -logtostderr=true

darwin:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s" -o kanary-operator github.com/etiennecoutaud/kanary/cmd/kanary-operator

linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s" -o kanary-operator github.com/etiennecoutaud/kanary/cmd/kanary-operator

release:
	docker build -t "$(REPO)/$(IMAGE):$(TAG)" .
	docker push "$(REPO)/$(IMAGE):$(TAG)"

test: 
	go test  $(shell go list ./... | grep -v fake) -coverprofile=coverage.txt -covermode=atomic

dep:
	glide up

gen:
	hack/update-codegen.sh

.PHONY: build test darwin image e2e clean-test update-version