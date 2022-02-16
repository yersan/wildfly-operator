package e2e

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	wildflyv1alpha1 "github.com/wildfly/wildfly-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("WildFly Basic tests", func() {
	BeforeEach(func() {
		if os.Getenv("LOCAL_MANAGER") == "0" {
			return
		}

		// create the name for testing
		testNamespace := &v1.Namespace{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}

		err := k8sClient.Create(context.Background(), testNamespace)
		Expect(err).ToNot(HaveOccurred())

		n := &v1.Namespace{}
		Eventually(func() error {
			return k8sClient.Get(context.Background(), types.NamespacedName{Name: namespace}, n)
		}, timeout, interval).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		if os.Getenv("LOCAL_MANAGER") == "0" {
			return
		}
		// clean up and delete the name for testing
		testNamespace := &v1.Namespace{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		err := k8sClient.Delete(context.Background(), testNamespace, client.PropagationPolicy(metav1.DeletePropagationForeground))
		Expect(err).ToNot(HaveOccurred())

		n := &v1.Namespace{}
		Eventually(func() bool {
			err = k8sClient.Get(context.Background(), types.NamespacedName{Name: namespace}, n)
			if err != nil && apierrors.IsNotFound(err) {
				return true
			}
			return false
		}, timeout, interval).Should(BeTrue())
	})

	It("WildFlyServer can be scale up", func() {
		var applicationImage = "quay.io/wildfly-quickstarts/wildfly-operator-quickstart:18.0"
		name := "example-wildfly"

		wildflyServer := MakeBasicWildFlyServer(namespace, name, applicationImage, 1, false)

		basicTest(name, wildflyServer)
	})

	It("WildFlyServer Bootable JAR can be scale up", func() {
		var applicationImage = "quay.io/wildfly-quickstarts/wildfly-operator-quickstart:bootable-21.0"
		name := "example-wildfly-bootable-jar"

		wildflyServer := MakeBasicWildFlyServer(namespace, name, applicationImage, 1, true)

		basicTest(name, wildflyServer)
	})
})

func basicTest(name string, wildflyServer *wildflyv1alpha1.WildFlyServer) {
	ctx := context.Background()
	log.Printf("Creating 1 replica")
	Expect(k8sClient.Create(ctx, wildflyServer)).Should(Succeed())
	stsLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
	statefulSet := &appsv1.StatefulSet{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, stsLookupKey, statefulSet)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Printf("Statefulset %s not found yet", name)
			}
			return false
		}
		log.Printf("Waiting for full availability of %s statefulset. Requested Replicas (%d) Ready (%d/%d)\n", name, 1, statefulSet.Status.ReadyReplicas, statefulSet.Status.Replicas)

		if statefulSet.Status.Replicas == 1 && statefulSet.Status.ReadyReplicas == 1 {
			return true
		}

		return false
	}, timeout, interval).Should(BeTrue())

	log.Printf("Scalling up to 2")
	changedDep := wildflyServer.DeepCopy()
	changedDep.Spec.Replicas = 2
	Expect(k8sClient.Patch(ctx, changedDep, client.MergeFrom(wildflyServer))).Should(Succeed())

	// Wait until the stateful set is ready
	stsLookupKey = types.NamespacedName{Name: name, Namespace: namespace}
	statefulSet = &appsv1.StatefulSet{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, stsLookupKey, statefulSet)
		if err != nil {
			return false
		}

		log.Printf("Waiting for full availability of %s statefulset. Requested Replicas (%d) Ready (%d/%d)\n", name, 2, statefulSet.Status.ReadyReplicas, statefulSet.Status.Replicas)

		if statefulSet.Status.Replicas == 2 && statefulSet.Status.ReadyReplicas == 2 {
			return true
		}
		return false
	}, timeout, interval).Should(BeTrue())
}
