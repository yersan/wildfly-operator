package e2e

import (
	wildflyv1alpha1 "github.com/wildfly/wildfly-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeBasicWildFlyServer creates a basic WildFlyServer resource
func MakeBasicWildFlyServer(ns, name, applicationImage string, size int32, bootableJar bool) *wildflyv1alpha1.WildFlyServer {
	return &wildflyv1alpha1.WildFlyServer{
		TypeMeta: metav1.TypeMeta{
			Kind:       "WildFlyServer",
			APIVersion: "wildfly.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: wildflyv1alpha1.WildFlyServerSpec{
			ApplicationImage: applicationImage,
			Replicas:         size,
			BootableJar:      bootableJar,
		},
	}
}
