HUB :=
REPO := etiennecoutaud
IMAGE := $(if $(HUB),$(HUB)/)$(REPO)/kanary-operator
TAG := $(shell git describe --tags --always)
TESTIMAGE :=

build:
	go build -i github.com/etiennecoutaud/kanary/cmd/kanary-operator

run: build
	kubectl apply -f manifests/kanary-crd.yml
	./kanary-operator -kubeconfig=$(HOME)/.kube/config -v=2

darwin:
	# Compile statically linked binary for darwin.
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s" -o kanary-operator github.com/etiennecoutaud/kanary/cmd/kanary-operator

image:
	docker build -t "$(IMAGE):$(TAG)" .

test:
	go test -v $(shell go list ./... | grep -v /vendor/ | grep -v /test/)

# requires minikube to be running
e2e:
	@if test 'x$(TESTIMAGE)' = 'x'; then echo "TESTIMAGE must be passed."; exit 1; fi
	go test -v ./test/e2e/ --image "$(TESTIMAGE)" --kubeconfig ~/.kube/config --ip "$$(minikube ip)"

clean-test:
	kubectl delete namespace testing
	kubectl delete clusterrolebinding habitat-operator
	kubectl delete clusterrole habitat-operator

dep:
	glide up

gen:
	hack/update-codegen.sh

.PHONY: build test darwin image e2e clean-test update-version