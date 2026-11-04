CHART_REPO_URL ?= http://example.com
HELM_REPO_DEST ?= /tmp/gh-pages
OPERATOR_NAME ?=$(shell basename -z `pwd`)
HELM_VERSION ?= v3.8.0
KIND_VERSION ?= v0.33.0
KO_VERSION ?= v0.19.1
KUBECTL_VERSION ?= v1.34.11
VAULT_VERSION ?= 1.9.3

# VERSION defines the project version used by the helm chart.
# Update this value when you upgrade the version of your project.
VERSION ?= 0.0.1

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.24.1

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

.PHONY: kind-setup
kind-setup: kind kubectl helm
	$(KIND) delete cluster
	$(KIND) create cluster --image docker.io/kindest/node:$(KUBECTL_VERSION) --config=./integration/cluster-kind.yaml
	$(HELM) upgrade ingress-nginx ./integration/helm/ingress-nginx -i --create-namespace -n ingress-nginx --atomic
	$(KUBECTL) wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=90s

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize kubectl ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize kubectl ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_TOOLS_VERSION ?= v0.23.0


.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: ko
KO = $(LOCALBIN)/ko
ko: $(KO) ## Download ko locally if necessary.
$(KO): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/google/ko@$(KO_VERSION)

# Generate helm chart
.PHONY: helmchart
helmchart: kustomize helm
	mkdir -p ./charts/${OPERATOR_NAME}/templates
	mkdir -p ./charts/${OPERATOR_NAME}/crds
	repo=${OPERATOR_NAME} envsubst < ./config/local-development/tilt/env-replace-image.yaml > ./config/local-development/tilt/replace-image.yaml
	$(KUSTOMIZE) build ./config/helmchart -o ./charts/${OPERATOR_NAME}/templates
	sed -i 's/release-namespace/{{.Release.Namespace}}/' ./charts/${OPERATOR_NAME}/templates/*.yaml
	rm ./charts/${OPERATOR_NAME}/templates/v1_namespace_release-namespace.yaml ./charts/${OPERATOR_NAME}/templates/apps_v1_deployment_${OPERATOR_NAME}-controller-manager.yaml
	mv ./charts/${OPERATOR_NAME}/templates/apiextensions.k8s.io_v1_customresourcedefinition* ./charts/${OPERATOR_NAME}/crds
	cp ./config/helmchart/templates/* ./charts/${OPERATOR_NAME}/templates
	version=${VERSION} envsubst < ./config/helmchart/Chart.yaml.tpl  > ./charts/${OPERATOR_NAME}/Chart.yaml
	version=${VERSION} image_repo=$${IMG%:*} envsubst < ./config/helmchart/values.yaml.tpl  > ./charts/${OPERATOR_NAME}/values.yaml
	sed -i '1s/^/{{ if .Values.enableMonitoring }}/' ./charts/${OPERATOR_NAME}/templates/monitoring.coreos.com_v1_servicemonitor_${OPERATOR_NAME}-controller-manager-metrics-monitor.yaml
	echo {{ end }} >> ./charts/${OPERATOR_NAME}/templates/monitoring.coreos.com_v1_servicemonitor_${OPERATOR_NAME}-controller-manager-metrics-monitor.yaml
	$(HELM) lint ./charts/${OPERATOR_NAME}	

.PHONY: helmchart-repo
	mkdir -p ${HELM_REPO_DEST}/${OPERATOR_NAME}
	$(HELM) package -d ${HELM_REPO_DEST}/${OPERATOR_NAME} ./charts/${OPERATOR_NAME}
	$(HELM) repo index --url ${CHART_REPO_URL} ${HELM_REPO_DEST}

.PHONY: helmchart-repo-push
helmchart-repo-push: helmchart-repo	
	git -C ${HELM_REPO_DEST} add .
	git -C ${HELM_REPO_DEST} status
	git -C ${HELM_REPO_DEST} commit -m "Release ${VERSION}"
	git -C ${HELM_REPO_DEST} push origin "gh-pages"	

HELM_TEST_IMG_TAG ?= helmchart-test

# Deploy the helmchart to a kind cluster to test deployment.
# If the test-metrics sidecar in the prometheus pod is ready, the metrics work and the test is successful.
.PHONY: helmchart-test
helmchart-test: kind-setup helmchart ko kind
	KO_DOCKER_REPO=ko.local $(KO) build --bare --tags=${HELM_TEST_IMG_TAG} .
	$(KIND) load docker-image ko.local:${HELM_TEST_IMG_TAG}
	$(HELM) repo add jetstack https://charts.jetstack.io
	$(HELM) install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace --version v1.7.1 --set installCRDs=true
	$(HELM) repo add prometheus-community https://prometheus-community.github.io/helm-charts
	$(HELM) install kube-prometheus-stack prometheus-community/kube-prometheus-stack -n default -f integration/kube-prometheus-stack-values.yaml
	$(HELM) install prometheus-rbac integration/helm/prometheus-rbac -n default
	$(HELM) upgrade -i ${OPERATOR_NAME}-local charts/${OPERATOR_NAME} -n ${OPERATOR_NAME}-local --create-namespace \
	  --set enableCertManager=true \
	  --set image.repository=ko.local \
	  --set image.tag=${HELM_TEST_IMG_TAG}
	$(KUBECTL) wait --namespace ${OPERATOR_NAME}-local --for=condition=ready pod --selector=app.kubernetes.io/name=${OPERATOR_NAME} --timeout=90s
	$(KUBECTL) wait --namespace default --for=condition=ready pod prometheus-kube-prometheus-stack-prometheus-0 --timeout=600s || { \
	  $(KUBECTL) get pods -A; \
	  $(KUBECTL) describe pod prometheus-kube-prometheus-stack-prometheus-0 -n default; \
	  $(KUBECTL) logs prometheus-kube-prometheus-stack-prometheus-0 -n default -c test-metrics --tail=30; \
	  exit 1; }
	$(KUBECTL) exec prometheus-kube-prometheus-stack-prometheus-0 -n default -c test-metrics -- /bin/sh -c "echo 'Example metrics...' && cat /tmp/ready"

.PHONY: kind
KIND = ./bin/kind
kind: ## Download kind locally if necessary.
ifeq (,$(wildcard $(KIND)))
ifeq (,$(shell which kind 2>/dev/null))
	$(call go-get-tool,$(KIND),sigs.k8s.io/kind@${KIND_VERSION})
else
KIND = $(shell which kind)
endif
endif

.PHONY: kubectl
KUBECTL = ./bin/kubectl
kubectl: ## Download kubectl locally if necessary.
ifeq (,$(wildcard $(KUBECTL)))
ifeq (,$(shell which kubectl 2>/dev/null))
	echo "Downloading ${KUBECTL} for managing k8s resources."
	OS=$(shell go env GOOS) ;\
	ARCH=$(shell go env GOARCH) ;\
	curl --create-dirs -sSLo ${KUBECTL} https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/$${OS}/$${ARCH}/kubectl ;\
	chmod +x ${KUBECTL}
else
KUBECTL = $(shell which kubectl)
endif
endif

.PHONY: helm
HELM = ./bin/helm
helm: ## Download helm locally if necessary.
ifeq (,$(wildcard $(HELM)))
ifeq (,$(shell which helm 2>/dev/null))
	echo "Downloading ${HELM}."
	OS=$(shell go env GOOS) ;\
	ARCH=$(shell go env GOARCH) ;\
	curl --create-dirs -sSLo ${HELM}.tar.gz https://get.helm.sh/helm-${HELM_VERSION}-$${OS}-$${ARCH}.tar.gz ;\
	tar -xf ${HELM}.tar.gz -C ./bin/ ;\
	mv ./bin/$${OS}-$${ARCH}/helm ${HELM}
else
HELM = $(shell which helm)
endif
endif
