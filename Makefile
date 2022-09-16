IMAGE_NAME := "aureq/cert-manager-webhook-ovh"
IMAGE_TAG := "latest"

.PHONY: rendered-manifest.yaml test build

OUT := $(shell pwd)/_out
TEST_ASSET_ETCD := $(OUT)/kubebuilder/bin/etcd
TEST_ASSET_KUBE_APISERVER := $(OUT)/kubebuilder/bin/kube-apiserver
TEST_ASSET_KUBECTL := $(OUT)/kubebuilder/bin/kubectl

$(shell mkdir -p "$(OUT)")

test:
	sh ./scripts/fetch-test-binaries.sh
	TEST_ASSET_ETCD="$(TEST_ASSET_ETCD)" TEST_ASSET_KUBE_APISERVER="$(TEST_ASSET_KUBE_APISERVER)" TEST_ASSET_KUBECTL="$(TEST_ASSET_KUBECTL)" \
	go test -v .

build:
	@test -z "$$HTTP_PROXY" -a -z "$$HTTPS_PROXY" || docker build --build-arg "HTTP_PROXY=$$HTTP_PROXY" --build-arg "HTTPS_PROXY=$$HTTPS_PROXY" -t "$(IMAGE_NAME):$(IMAGE_TAG)" .
	@test ! -z "$$HTTP_PROXY" -o ! -z "$$HTTPS_PROXY" || docker build -t "$(IMAGE_NAME):$(IMAGE_TAG)" .

rendered-manifest.yaml:
	helm template \
	    cert-manager-webhook-ovh \
        --set image.repository=$(IMAGE_NAME) \
        --set image.tag=$(IMAGE_TAG) \
        charts/cert-manager-webhook-ovh > "$(OUT)/rendered-manifest.yaml"
