package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"runtime/pprof"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kube-state-metrics/v2/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/v2/pkg/options"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
)

const (
	testdataPath = "test/benchmarks/kubernetes_state/testdata"
)

func openOrDie(name string) (file *os.File) {
	fullFilename := path.Join(testdataPath, name)
	file, err := os.Open(fullFilename)
	if err != nil {
		if os.IsNotExist(err) {
			log.Fatalf("\"%s\" doesn’t exist. Have you run \"%s\"?\n", fullFilename, path.Join(testdataPath, "generate.sh"))
		}
		log.Fatalf("Error while opening \"%s\": %v\n", fullFilename, err)
	}
	return
}

func main() {
	fakeClient := fake.NewSimpleClientset()

	/*
	 * Populate fake client with Namespaces
	 */
	file := openOrDie("namespaces.json")
	var namespaceList corev1.NamespaceList
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&namespaceList); err != nil {
		log.Fatalf("Error while decoding namespace list: %v\n", err)
	}
	file.Close()

	for _, namespace := range namespaceList.Items {
		fakeClient.CoreV1().Namespaces().Create(&namespace)
	}

	/*
	 * Populate fake client with Pods
	 */
	file = openOrDie("pods.json")
	var podList corev1.PodList
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&podList); err != nil {
		log.Fatalf("Error while decoding pod list: %v\n", err)
	}
	file.Close()

	for _, pod := range podList.Items {
		fakeClient.CoreV1().Pods(pod.Namespace).Create(&pod)
	}

	/*
	 * Populate fake client with Services
	 */
	file = openOrDie("services.json")
	var serviceList corev1.ServiceList
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&serviceList); err != nil {
		log.Fatalf("Error while decoding service list: %v\n", err)
	}
	file.Close()

	for _, service := range serviceList.Items {
		fakeClient.CoreV1().Services(service.Namespace).Create(&service)
	}

	/*
	 * Populate fake client with DaemonSets
	 */
	file = openOrDie("daemonsets.json")
	var daemonSetList appsv1.DaemonSetList
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&daemonSetList); err != nil {
		log.Fatalf("Error while decoding daemon set list: %v\n", err)
	}
	file.Close()

	for _, daemonSet := range daemonSetList.Items {
		fakeClient.AppsV1().DaemonSets(daemonSet.Namespace).Create(&daemonSet)
	}

	/*
	 * Populate fake client with Deployments
	 */
	file = openOrDie("deployments.json")
	var deploymentList appsv1.DeploymentList
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&deploymentList); err != nil {
		log.Fatalf("Error while decoding deployment list: %v\n", err)
	}
	file.Close()

	for _, deployment := range deploymentList.Items {
		fakeClient.AppsV1().Deployments(deployment.Namespace).Create(&deployment)
	}

	/*
	 * Populate fake client with StatefulSets
	 */
	file = openOrDie("statefulsets.json")
	var statefulSetList appsv1.StatefulSetList
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&statefulSetList); err != nil {
		log.Fatalf("Error while decoding stateful set list: %v\n", err)
	}
	file.Close()

	for _, statefulSet := range statefulSetList.Items {
		fakeClient.AppsV1().StatefulSets(statefulSet.Namespace).Create(&statefulSet)
	}

	/*
	 * Populate fake client with Jobs
	 */
	file = openOrDie("jobs.json")
	var jobList batchv1.JobList
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&jobList); err != nil {
		log.Fatalf("Error while decoding job list: %v\n", err)
	}
	file.Close()

	for _, job := range jobList.Items {
		fakeClient.BatchV1().Jobs(job.Namespace).Create(&job)
	}

	/*
	 * Create a mock store
	 */
	builder := kubestatemetrics.New()
	builder.WithEnabledResources(options.DefaultResources.AsSlice())
	builder.WithNamespaces(options.DefaultNamespaces)
	allowDenyList, err := allowdenylist.New(options.MetricSet{}, nil)
	if err != nil {
		log.Fatalf("allowdenylist.New(…) failed: %v\n", err)
	}
	if err := allowDenyList.Parse(); err != nil {
		log.Fatalf("allowDenyList.Parse() failed: %v\n", err)
	}
	builder.WithAllowDenyList(allowDenyList)
	builder.WithKubeClient(fakeClient)
	ctx, cancel := context.WithCancel(context.Background())
	builder.WithContext(ctx)
	builder.WithResync(1 * time.Second)
	builder.WithGenerateStoreFunc(builder.GenerateStore)

	store := builder.Build()

	/*
	 * Create the KSMCheck
	 */
	labelsMapper := map[string]string{
		"label_app":            "app",
		"label_chart_name":     "chart_name",
		"label_chart_version":  "chart_version",
		"label_consumer_group": "consumer_group",
		"label_kafka_topic":    "kafka_topic",
		"label_logs_team":      "logs_team",
		"label_service":        "service",
		"label_team":           "team",
	}

	labelJoins := map[string]*cluster.JoinsConfig{
		"kube_daemonset_labels": &cluster.JoinsConfig{
			LabelsToMatch: []string{"daemonset", "namespace"},
			LabelsToGet:   []string{"label_service", "label_chart_name", "label_chart_version", "label_team", "label_app"},
		},
		"kube_deployment_labels": &cluster.JoinsConfig{
			LabelsToMatch: []string{"deployment", "namespace"},
			LabelsToGet:   []string{"label_service", "label_chart_name", "label_chart_version", "label_team", "label_logs_team", "label_kafka_topic", "label_consumer_group", "label_app"},
		},
		"kube_job_labels": &cluster.JoinsConfig{
			LabelsToMatch: []string{"job_name", "namespace"},
			LabelsToGet:   []string{"label_service", "label_chart_name", "label_chart_version", "label_team", "label_logs_team", "label_app"},
		},
		"kube_statefulset_labels": &cluster.JoinsConfig{
			LabelsToMatch: []string{"statefulset", "namespace"},
			LabelsToGet:   []string{"label_service", "label_chart_name", "label_chart_version", "label_team", "label_logs_team", "label_kafka_topic", "label_consumer_group", "label_app"},
		},
	}

	kubeStateMetricsCheck := cluster.KubeStateMetricsFactoryWithParam(labelsMapper, labelJoins, store)

	/*
	 * Initialize the aggregator
	 * As it has a `nil` serializer, it will panic if it tries to flush the metrics.
	 * That’s why we need a big enough flush interval
	 */
	aggregator.InitAggregatorWithFlushInterval(nil, "", 1*time.Hour)

	/*
	 * Wait for informers to get populated
	 */
	time.Sleep(2 * time.Second)

	/*
	 * Call and benchmark KSMCheck.Run()
	 */
	file, err = os.Create("cpuprofile.pprof")
	if err != nil {
		log.Printf("Failed to create \"cpuprofile.pprof\": %v\n", err)
		return
	}
	defer file.Close()

	pprof.StartCPUProfile(file)
	start := time.Now()
	err = kubeStateMetricsCheck.Run()
	elapsed := time.Since(start)
	pprof.StopCPUProfile()

	cancel()

	fmt.Printf("KSMCheck.Run() returned %v in %s\n", err, elapsed)
}
