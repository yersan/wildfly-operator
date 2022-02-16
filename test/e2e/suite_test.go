/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"github.com/RHsyseng/operator-utils/pkg/utils/openshift"
	"github.com/onsi/gomega/gexec"
	route "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	wildflyv1alpha1 "github.com/wildfly/wildfly-operator/api/v1alpha1"
	"github.com/wildfly/wildfly-operator/controllers"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"log"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
)

const (
	timeout   = time.Minute * 5
	duration  = time.Minute * 5
	interval  = time.Second * 2
	namespace = "wildfly-op-test-ns"
)

func TestAPIs(t *testing.T) {
	if os.Getenv("SKIP_E2E") == "1" {
		t.Skip("Skipping E2E tests")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx := context.TODO()

	By("bootstrapping test environment")
	if os.Getenv("LOCAL_MANAGER") == "0" {

		log.Printf("Start testing deploying the Operator")
		// We expect are deploying the Operator
		testEnv = &envtest.Environment{
			UseExistingCluster:       pointer.BoolPtr(true),
			ErrorIfCRDPathMissing:    true,
			AttachControlPlaneOutput: true,
		}

		cfg, k8sClient = initialSetup()

		// Wait until the Operator gets deployed
		log.Printf("Waiting for full availability of the wildfly-operator")
		operatorKey := types.NamespacedName{Name: "wildfly-operator", Namespace: "wildfly-op-test-ns"}
		wflyDeployment := &appsv1.Deployment{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, operatorKey, wflyDeployment)
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.Printf("wildfly-operator not found.")
				}
				return false
			}

			log.Printf("Waiting for full availability of %s resource. Requested Replicas (%d) Ready (%d/%d)\n", "wildfly-operator", 1, wflyDeployment.Status.ReadyReplicas, wflyDeployment.Status.Replicas)
			if wflyDeployment.Status.ReadyReplicas == 1 {
				return true
			}

			return false
		}, timeout, interval).Should(BeTrue())

	} else {

		// When we are running the test in local, we do not deploy the Operator
		// web run the manager directly from the test suite
		testEnv = &envtest.Environment{
			UseExistingCluster: pointer.BoolPtr(true),
			CRDDirectoryPaths: []string{
				filepath.Join("..", "..", "config", "crd", "bases"),
			},
			ErrorIfCRDPathMissing:    true,
			AttachControlPlaneOutput: true,
		}

		cfg, k8sClient = initialSetup()

		os.Setenv("JBOSS_HOME", "/wildfly")
		os.Setenv("JBOSS_BOOTABLE_DATA_DIR", "/opt/jboss/container/wildfly-bootable-jar-data")
		os.Setenv("OPERATOR_NAME", "wildfly-operator")
		os.Setenv("JBOSS_BOOTABLE_HOME", "/opt/jboss/container/wildfly-bootable-jar-server")

		// start the manager and reconciler
		k8sManager, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme.Scheme,
			MetricsBindAddress:     "0.0.0.0:8383",
			Port:                   9443,
			HealthProbeBindAddress: ":8081",
			LeaderElection:         false,
		})
		Expect(err).ToNot(HaveOccurred())

		isOpenShift, err := openshift.IsOpenShift(cfg)
		Expect(err).ToNot(HaveOccurred())
		err = (&controllers.WildFlyServerReconciler{
			Client:      k8sManager.GetClient(),
			Scheme:      k8sManager.GetScheme(),
			Recorder:    k8sManager.GetEventRecorderFor("wildflyserver-controller"),
			Log:         ctrl.Log.WithName("test").WithName("WildFlyServer"),
			IsOpenShift: isOpenShift,
		}).SetupWithManager(k8sManager)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			err = k8sManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()
	}
})

func initialSetup() (*rest.Config, client.Client) {
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register resources in the schema
	err = wildflyv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = monitoringv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = route.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	return cfg, k8sClient
}

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	gexec.KillAndWait(5 * time.Second)
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
