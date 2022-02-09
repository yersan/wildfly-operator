module github.com/wildfly/wildfly-operator

go 1.15

require (
	github.com/RHsyseng/operator-utils v1.4.6 // indirect
	github.com/go-logr/logr v0.3.0
	github.com/go-openapi/spec v0.19.9
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/openshift/api v0.0.0-20210928121311-b64fe3d0dc32 // indirect
	github.com/prometheus-operator/prometheus-operator v0.44.1 // indirect
	github.com/tevino/abool v1.2.0 // indirect
	k8s.io/api v0.19.14
	k8s.io/apimachinery v0.19.14
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20200805222855-6aeccd4b50c6
	sigs.k8s.io/controller-runtime v0.7.0
)

replace (
	k8s.io/client-go => k8s.io/client-go v0.19.14
)
