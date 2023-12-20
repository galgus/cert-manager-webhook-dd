IMAGE_NAME := "k41374/cert-manager-webhook-dd"
IMAGE_TAG := "1.0.7"

.PHONY: rendered-manifest.yaml test build

OUT := $(shell pwd)/_out
TEST_ASSET_ETCD := $(OUT)/kubebuilder/bin/etcd
TEST_ASSET_KUBE_APISERVER := $(OUT)/kubebuilder/bin/kube-apiserver
TEST_ASSET_KUBECTL := $(OUT)/kubebuilder/bin/kubectl

test:
	@test -d "$(OUT)" || mkdir -p "$(OUT)"
	@sh ./scripts/fetch-test-binaries.sh
	TEST_ASSET_ETCD="$(TEST_ASSET_ETCD)" \
		TEST_ASSET_KUBE_APISERVER="$(TEST_ASSET_KUBE_APISERVER)" \
		TEST_ASSET_KUBECTL="$(TEST_ASSET_KUBECTL)" \
		go test -v .

build:
	@test -z "$$HTTP_PROXY" -a -z "$$HTTPS_PROXY" || docker build \
		--build-arg "HTTP_PROXY=$$HTTP_PROXY" \
		--build-arg "HTTPS_PROXY=$$HTTPS_PROXY" \
		-t "$(IMAGE_NAME):$(IMAGE_TAG)" .
	@test ! -z "$$HTTP_PROXY" -o ! -z "$$HTTPS_PROXY" || docker build \
		-t "$(IMAGE_NAME):$(IMAGE_TAG)" .

rendered-manifest.yaml:
	@test -d "$(OUT)" || mkdir -p "$(OUT)"
	@helm template \
	    cert-manager-webhook-dd \
        --set image.repository=$(IMAGE_NAME) \
        --set image.tag=$(IMAGE_TAG) \
        charts/cert-manager-webhook-dd > "$(OUT)/rendered-manifest.yaml"
