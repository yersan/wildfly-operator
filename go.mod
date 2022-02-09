module github.com/wildfly/wildfly-operator

go 1.15

require (
	github.com/go-logr/logr v0.3.0
	github.com/go-openapi/spec v0.19.3
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	k8s.io/api v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v0.19.4
	k8s.io/kube-openapi v0.0.0-20200805222855-6aeccd4b50c6
	sigs.k8s.io/controller-runtime v0.7.0
)

replace k8s.io/client-go => k8s.io/client-go v0.19.4
