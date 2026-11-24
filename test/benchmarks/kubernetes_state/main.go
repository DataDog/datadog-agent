// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"runtime/pprof"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kube-state-metrics/v2/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/v2/pkg/options"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	cluster "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"
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
	// The benchmark binary is built with the "test" tag,
	// which unconditionally sets up a debug-level logger.
	// Set it to info to keep it quiet.
	ddlog.SetupLogger(ddlog.Default(), "info")

	// fake.NewSimpleClientset() never sends the bookmark event that the watch-list
	// initial sync (client-go's default since v0.35) requires, so reflectors would
	// otherwise stall for the full randomized minWatchTimeout (5-10 minutes) before
	// falling back to a regular List(). Must be set before any reflector starts.
	os.Setenv("KUBE_FEATURE_WatchListClient", "false")

	ctx, cancel := context.WithCancel(context.Background())
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
		fakeClient.CoreV1().Namespaces().Create(ctx, &namespace, metav1.CreateOptions{})
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
		fakeClient.CoreV1().Pods(pod.Namespace).Create(ctx, &pod, metav1.CreateOptions{})
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
		fakeClient.CoreV1().Services(service.Namespace).Create(ctx, &service, metav1.CreateOptions{})
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
		fakeClient.AppsV1().DaemonSets(daemonSet.Namespace).Create(ctx, &daemonSet, metav1.CreateOptions{})
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
		fakeClient.AppsV1().Deployments(deployment.Namespace).Create(ctx, &deployment, metav1.CreateOptions{})
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
		fakeClient.AppsV1().StatefulSets(statefulSet.Namespace).Create(ctx, &statefulSet, metav1.CreateOptions{})
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
		fakeClient.BatchV1().Jobs(job.Namespace).Create(ctx, &job, metav1.CreateOptions{})
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
	builder.WithFamilyGeneratorFilter(allowDenyList)
	builder.WithKubeClient(fakeClient)
	builder.WithContext(ctx)
	builder.WithGenerateStoresFunc(builder.GenerateStores)

	store := builder.BuildStores()

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

	labelJoins := map[string]*cluster.JoinsConfigWithoutLabelsMapping{
		"kube_daemonset_labels": {
			LabelsToMatch: []string{"daemonset", "namespace"},
			LabelsToGet:   []string{"label_service", "label_chart_name", "label_chart_version", "label_team", "label_app"},
		},
		"kube_deployment_labels": {
			LabelsToMatch: []string{"deployment", "namespace"},
			LabelsToGet:   []string{"label_service", "label_chart_name", "label_chart_version", "label_team", "label_logs_team", "label_kafka_topic", "label_consumer_group", "label_app"},
		},
		"kube_job_labels": {
			LabelsToMatch: []string{"job_name", "namespace"},
			LabelsToGet:   []string{"label_service", "label_chart_name", "label_chart_version", "label_team", "label_logs_team", "label_app"},
		},
		"kube_statefulset_labels": {
			LabelsToMatch: []string{"statefulset", "namespace"},
			LabelsToGet:   []string{"label_service", "label_chart_name", "label_chart_version", "label_team", "label_logs_team", "label_kafka_topic", "label_consumer_group", "label_app"},
		},
	}

	taggerComponent := nooptagger.NewComponent()

	kubeStateMetricsCheck := cluster.KubeStateMetricsFactoryWithParam(labelsMapper, labelJoins, store, taggerComponent)

	/*
	 * The check's Run() needs a working sender.SenderManager.
	 * A no-op stub (rather than a real aggregator/demultiplexer) keeps the
	 * profile focused on the check's own logic instead of aggregator-side
	 * bookkeeping (channel hop, context resolver, retained series).
	 */
	var senderManager noopSenderManager
	if err := kubeStateMetricsCheck.CommonConfigure(&senderManager, nil, nil, "", ""); err != nil {
		log.Fatalf("Failed to configure the sender manager on the check: %v\n", err)
	}

	/*
	 * Wait for informers to get populated
	 * TODO: wait for the initial reflector sync instead of a fixed sleep.
	 */
	time.Sleep(5 * time.Second)

	/*
	 * Call and benchmark KSMCheck.Run()
	 */
	file, err = os.Create("cpuprofile.pprof")
	if err != nil {
		log.Printf("Failed to create \"cpuprofile.pprof\": %v\n", err)
		return
	}

	/*
	 * alloc_space/alloc_objects are cumulative since process start.
	 * Snapshot the heap immediately before and after Run() so callers can diff the two
	 * via `go tool pprof -alloc_space -diff_base=heap_before.pprof heap_after.pprof`
	 * and isolate what Run() itself allocated.
	 */
	writeHeapProfile("heap_before.pprof")

	pprof.StartCPUProfile(file)
	start := time.Now()
	err = kubeStateMetricsCheck.Run()
	elapsed := time.Since(start)
	pprof.StopCPUProfile()

	writeHeapProfile("heap_after.pprof")

	cancel()
	fmt.Printf("KSMCheck.Run() returned %v in %s\n", err, elapsed)
	fmt.Printf("	unknown metric families: %.0f\n", senderManager.sender.unknownMetricsCount)
	fmt.Printf("	total metrics: %.0f\n", senderManager.sender.totalMetricsCount)

	if err = file.Close(); err != nil {
		log.Printf("failed to close \"cpuprofile.pprof\": %v\n", err)
		return
	}
}

// writeHeapProfile forces a GC so inuse_space/inuse_objects reflect only
// still-referenced data, then writes both the inuse and cumulative alloc
// counters for the current point in the program to name.
func writeHeapProfile(name string) {
	runtime.GC()
	f, err := os.Create(name)
	if err != nil {
		log.Printf("Failed to create %q: %v\n", name, err)
		return
	}
	if err := pprof.WriteHeapProfile(f); err != nil {
		log.Printf("Failed to write heap profile to %q: %v\n", name, err)
	}
	if err := f.Close(); err != nil {
		log.Printf("failed to close %q: %v\n", name, err)
	}
}
