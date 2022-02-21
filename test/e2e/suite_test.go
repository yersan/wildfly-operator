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
	"bytes"
	"context"
	"fmt"
	"github.com/RHsyseng/operator-utils/pkg/utils/openshift"
	"github.com/onsi/gomega/gexec"
	route "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	wildflyv1alpha1 "github.com/wildfly/wildfly-operator/api/v1alpha1"
	"github.com/wildfly/wildfly-operator/controllers"
	"io"
	"io/ioutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"log"
	"os"
	"path/filepath"
	"regexp"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"strings"
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
	cfg            *rest.Config
	k8sClient      client.Client
	testEnv        *envtest.Environment
	operator       *appsv1.Deployment
	serviceAccount *corev1.ServiceAccount
	role           *rbacv1.Role
	roleBinding    *rbacv1.RoleBinding
)

const (
	timeout   = time.Minute * 5
	interval  = time.Second * 2
	duration  = time.Second * 20
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
		testEnv = &envtest.Environment{
			UseExistingCluster:       pointer.BoolPtr(true),
			CRDDirectoryPaths:        []string{filepath.Join("..", "..", "config", "crd", "bases")},
			CRDInstallOptions:        envtest.CRDInstallOptions{CleanUpAfterUse: true},
			ErrorIfCRDPathMissing:    true,
			AttachControlPlaneOutput: true,
		}

		cfg, k8sClient = initialSetup()

		// load resources for tests generated by kustomize
		data, err := ioutil.ReadFile("../../dry-run/test-resources.yaml")
		if err != nil {
			log.Fatal(err)
		}

		decode := scheme.Codecs.UniversalDeserializer().Decode
		for _, f := range strings.Split(string(data), "---") {
			obj, gKV, _ := decode([]byte(f), nil, nil)
			switch gKV.Kind {
			case "Deployment":
				operator = obj.(*appsv1.Deployment)
			case "ServiceAccount":
				serviceAccount = obj.(*corev1.ServiceAccount)
			case "Role":
				role = obj.(*rbacv1.Role)
			case "RoleBinding":
				roleBinding = obj.(*rbacv1.RoleBinding)
			default:
				// unexpected type, ignore
			}
		}

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

// MakeBasicWildFlyServerWithStorage creates a basic WildFlyServer resource configured with a persistent storage assuming it will be provisioned
// dynamically by the cluster provider
func MakeBasicWildFlyServerWithStorage(ns, name, applicationImage string, size int32, bootableJar bool) *wildflyv1alpha1.WildFlyServer {
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
			Storage: &wildflyv1alpha1.StorageSpec{
				VolumeClaimTemplate: corev1.PersistentVolumeClaim{
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("5Mi"),
							},
						},
					},
				},
			},
		},
	}
}

func WaitUntilReady(ctx context.Context, k8sClient client.Client, server *wildflyv1alpha1.WildFlyServer) {
	name := server.Name
	ns := server.Namespace
	size := server.Spec.Replicas

	stsLookupKey := types.NamespacedName{Name: name, Namespace: ns}
	statefulSet := &appsv1.StatefulSet{}
	log.Printf("Creating %s WildFly Server resource.", name)
	Eventually(func() bool {
		err := k8sClient.Get(ctx, stsLookupKey, statefulSet)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Printf("Resource %s not found.", name)
			}
			return false
		}

		log.Printf("Waiting for full availability of %s statefulset. Requested Replicas (%d) Ready (%d/%d)\n", name, size, statefulSet.Status.ReadyReplicas, statefulSet.Status.Replicas)

		if statefulSet.Status.Replicas == size && statefulSet.Status.ReadyReplicas == size {
			return true
		}
		return false
	}, timeout, interval).Should(BeTrue())
}

func WaitUntilDeploymentReady(ctx context.Context, k8sClient client.Client, deployment *appsv1.Deployment) {
	name := deployment.Name
	ns := deployment.Namespace
	size := deployment.Spec.Replicas

	stsLookupKey := types.NamespacedName{Name: name, Namespace: ns}
	d := &appsv1.Deployment{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, stsLookupKey, d)
		if err != nil {
			log.Printf("Cannot get the deployment %s from namespace %s", name, ns)
			return false
		}

		log.Printf("Waiting for full availability of %s statefulset. Requested Replicas (%d) Ready (%d/%d)\n", name, *size, d.Status.ReadyReplicas, d.Status.Replicas)

		if d.Status.Replicas == *size && d.Status.ReadyReplicas == *size {
			return true
		}
		return false
	}, timeout, interval).Should(BeTrue())
}

func WaitUntilServerDeleted(ctx context.Context, k8sClient client.Client, server *wildflyv1alpha1.WildFlyServer) {
	name := server.Name
	ns := server.Namespace

	log.Printf("Deleting %s WildFly Server resource", name)
	err := k8sClient.Delete(context.Background(), server, client.PropagationPolicy(metav1.DeletePropagationBackground))
	Expect(err).ToNot(HaveOccurred())

	stsLookupKey := types.NamespacedName{Name: name, Namespace: ns}
	statefulSet := &appsv1.StatefulSet{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, stsLookupKey, statefulSet)
		if err != nil && apierrors.IsNotFound(err) {
			log.Printf("Resource %s deleted", name)
			return true
		}

		log.Printf("Waiting until resource %s is deleted", name)
		return false
	}, timeout, interval).Should(BeTrue())
}

// WaitUntilClusterIsFormed wait until a cluster is formed with all the podNames
func WaitUntilClusterIsFormed(server *wildflyv1alpha1.WildFlyServer, podName1 string, podName2 string) {
	pattern := fmt.Sprintf(".*ISPN000094: Received new cluster view.*(.*%s, .*%s|.*%[2]s, .*%[1]s).*", podName1, podName2)
	Eventually(func() bool {
		var clusterFormed bool
		for _, podName := range []string{podName1, podName2} {
			logs, err := GetLogs(server, podName)
			if err != nil {
				log.Printf("[%v] Can't get log for %s. Probably waiting for the container being started "+
					"(e.g. pod could be still in state 'ContainerCreating'), error: %v", time.Now().String(), podName, err)
				return false
			}

			match, _ := regexp.MatchString(pattern, logs)

			if match {
				clusterFormed = true
				log.Printf("got cluster view log in %s", podName)
			} else {
				clusterFormed = false
				log.Printf("Waiting for cluster view log in %s", podName)
				log.Printf(logs)
			}
		}
		return clusterFormed
	}, timeout, interval).Should(BeTrue())

	log.Printf("Cluster view formed with %s & %s", podName1, podName2)
}

// GetLogs returns the logs from the given pod (in the server's namespace).
func GetLogs(server *wildflyv1alpha1.WildFlyServer, podName string) (string, error) {
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "Error getting los", err
	}

	logsReq := clientSet.CoreV1().Pods(server.ObjectMeta.Namespace).GetLogs(podName, &corev1.PodLogOptions{})
	podLogs, err := logsReq.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", err
	}
	logs := buf.String()
	return logs, nil
}
