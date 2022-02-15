package e2e_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/wildfly/wildfly-operator/test/e2e"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"time"
)

var _ = Describe("WIldFly test", func() {
	const (
		applicationImage = "quay.io/wildfly-quickstarts/wildfly-operator-quickstart:18.0"
		timeout          = time.Minute * 5
		duration         = time.Minute * 5
		interval         = time.Millisecond * 250
	)

	var namespace = "test-operator-ns" + unixEpoch()

	BeforeEach(func() {
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
		// clean up and delete the name for testing
		testNamespace := &v1.Namespace{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		err := k8sClient.Delete(context.Background(), testNamespace, client.PropagationPolicy(metav1.DeletePropagationForeground))
		Expect(err).ToNot(HaveOccurred())

		n := &v1.Namespace{}
		Eventually(func() error {
			return k8sClient.Get(context.Background(), types.NamespacedName{Name: namespace}, n)
		}, timeout, interval).ShouldNot(HaveOccurred())
	})

	Describe("WildFly Basic Server Scale Test", func() {
		It("Should deploy a S2I application image", func() {
			ctx := context.Background()
			name := "example-wildfly-" + unixEpoch()
			wildflyserver := e2e.MakeBasicWildFlyServer(namespace, name, applicationImage, 1, false)

			Expect(k8sClient.Create(ctx, wildflyserver)).Should(Succeed())

			// Wait until it is created
			wflyLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, wflyLookupKey, wildflyserver)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			log.Printf("Application %s is deployed with %d instance\n", name, 1)

			// Wait until have a StatefulSet with one replica
			stsLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, stsLookupKey, statefulSet)
				if err != nil {
					return false
				}

				if statefulSet.Status.Replicas == 1 && statefulSet.Status.ReadyReplicas == 1 {
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())
			log.Printf("First replica created")

			log.Printf("Scalling it up to 2")
			Expect(k8sClient.Get(ctx, wflyLookupKey, wildflyserver)).Should(Succeed())
			wildflyserver.Spec.Replicas = 2
			Expect(k8sClient.Update(ctx, wildflyserver)).Should(Succeed())

			// Wait until the stateful set is ready
			stsLookupKey = types.NamespacedName{Name: name, Namespace: namespace}
			statefulSet = &appsv1.StatefulSet{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, stsLookupKey, statefulSet)
				if err != nil {
					return false
				}

				if statefulSet.Status.Replicas == 2 && statefulSet.Status.ReadyReplicas == 2 {
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})

func unixEpoch() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
