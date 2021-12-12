module github.com/redhat-cop/patch-operator

go 1.16

require (
	github.com/evanphx/json-patch v5.6.0+incompatible
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.com/redhat-cop/operator-utils v2.0.0+incompatible
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	sigs.k8s.io/controller-runtime v0.10.0
	sigs.k8s.io/yaml v1.3.0
)
