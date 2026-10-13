// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// Buffers are large enough for all the events of a test so the watch never blocks.
const terminatedPodTestBufferSize = 10

type TerminatedPodCollectorTestSuite struct {
	suite.Suite
	collector   *TerminatedPodCollector
	fakeWatcher *watch.FakeWatcher
	handledPods chan *corev1.Pod
	runCfg      *collectors.CollectorRunConfig
	stopCh      chan struct{}
}

func (s *TerminatedPodCollectorTestSuite) SetupTest() {
	// The processor skips resources already seen in this global cache.
	orchestrator.KubernetesResourceCache.Flush()

	mockStore := fxutil.Test[workloadmetamock.Mock](s.T(), fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	s.collector = NewTerminatedPodCollector(mockconfig.New(s.T()), mockStore, taggerfxmock.SetupFakeTagger(s.T()))
	s.fakeWatcher = watch.NewFakeWithOptions(watch.FakeOptions{ChannelSize: terminatedPodTestBufferSize})
	s.handledPods = make(chan *corev1.Pod, terminatedPodTestBufferSize)
	s.stopCh = make(chan struct{})

	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		podList := &corev1.PodList{
			ListMeta: metav1.ListMeta{ResourceVersion: "1"},
		}
		return true, podList, nil
	})
	client.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, s.fakeWatcher, nil
	})

	orchestratorCfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	orchestratorCfg.KubeClusterName = "test-cluster"

	s.runCfg = &collectors.CollectorRunConfig{
		K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
			APIClient: &apiserver.APIClient{Cl: client},
		},
		ClusterID:                 "test-cluster",
		Config:                    orchestratorCfg,
		MsgGroupRef:               atomic.NewInt32(0),
		StopCh:                    s.stopCh,
		TerminatedResourceHandler: s.handleTerminatedResource,
	}

	s.collector.Init(s.runCfg)
}

func (s *TerminatedPodCollectorTestSuite) TearDownTest() {
	close(s.stopCh)
	s.collector.watcher.AwaitStop()
}

func (s *TerminatedPodCollectorTestSuite) TestCollect_Success_NoPodProcessedForAddedPod() {
	pod := newTerminatedPodTestPod("added-pod", "uid-1", "1")

	result := s.processEvents(0, watch.Event{Type: watch.Added, Object: pod})

	s.Equal(newEmptyTerminatedPodTestResult(), result)
}

func (s *TerminatedPodCollectorTestSuite) TestCollect_Success_NoPodProcessedForModifiedPod() {
	pod := newTerminatedPodTestPod("modified-pod", "uid-2", "2")

	result := s.processEvents(0, watch.Event{Type: watch.Modified, Object: pod})

	s.Equal(newEmptyTerminatedPodTestResult(), result)
}

func (s *TerminatedPodCollectorTestSuite) TestCollect_Success_PodForceDeletionIsProcessed() {
	pod := newTerminatedPodTestPod("force-deleted-pod", "uid-3", "3")
	pod.DeletionTimestamp = pointer.Ptr(metav1.NewTime(time.Date(2021, time.April, 16, 15, 30, 0, 0, time.UTC)))
	pod.DeletionGracePeriodSeconds = pointer.Ptr(int64(0))

	result := s.processEvents(1, watch.Event{Type: watch.Deleted, Object: pod})

	expected := newTerminatedPodTestResult(s.T(), pod, "Terminating", "3236024791948317692")
	s.Equal(expected, result)
}

func (s *TerminatedPodCollectorTestSuite) TestCollect_Success_PodRegularDeletionIsProcessed() {
	pod := newTerminatedPodTestPod("deleted-pod", "uid-4", "4")
	pod.DeletionTimestamp = pointer.Ptr(metav1.NewTime(time.Date(2021, time.April, 16, 15, 30, 0, 0, time.UTC)))
	pod.DeletionGracePeriodSeconds = pointer.Ptr(int64(0))
	pod.Status.Phase = corev1.PodSucceeded

	result := s.processEvents(1, watch.Event{Type: watch.Deleted, Object: pod})

	expected := newTerminatedPodTestResult(s.T(), pod, "Succeeded", "9484515160463151052")
	s.Equal(expected, result)
}

func (s *TerminatedPodCollectorTestSuite) handleTerminatedResource(k8sCollector collectors.K8sCollector, obj any) {
	s.Same(s.collector, k8sCollector)

	pod, ok := obj.(*corev1.Pod)
	if !s.True(ok, "terminated resource must be a pod, got %T", obj) {
		return
	}

	s.handledPods <- pod
}

// processEvents sends events to the collector watch and processes the pods it hands over.
// Pods are processed in a single batch, like the terminated resource bundle does.
func (s *TerminatedPodCollectorTestSuite) processEvents(expectedPods int, events ...watch.Event) *collectors.CollectorRunResult {
	for _, event := range events {
		pod, ok := event.Object.(*corev1.Pod)
		s.Require().True(ok, "event object must be a pod, got %T", event.Object)

		s.fakeWatcher.Action(event.Type, pod)
	}

	if expectedPods == 0 {
		hasHandledPods := func() bool { return len(s.handledPods) > 0 }
		s.Require().Never(hasHandledPods, 5*time.Second, 100*time.Millisecond)
	} else {
		hasExpectedPods := func() bool { return len(s.handledPods) == expectedPods }
		s.Require().Eventually(hasExpectedPods, 5*time.Second, 100*time.Millisecond)
	}

	pods := make([]*corev1.Pod, 0, expectedPods)
	for range expectedPods {
		pods = append(pods, <-s.handledPods)
	}

	result, err := s.collector.Process(s.runCfg, pods)
	s.Require().NoError(err)

	return result
}

func TestTerminatedPodCollector(t *testing.T) {
	suite.Run(t, new(TerminatedPodCollectorTestSuite))
}

func newTerminatedPodTestPod(name string, uid types.UID, resourceVersion string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: CreateTestTime(),
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:            name,
			Namespace:       "default",
			ResourceVersion: resourceVersion,
			UID:             uid,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

func newEmptyTerminatedPodTestResult() *collectors.CollectorRunResult {
	return &collectors.CollectorRunResult{
		Result: processors.ProcessResult{
			MetadataMessages: []model.MessageBody{},
			ManifestMessages: []model.MessageBody{},
		},
	}
}

// newTerminatedPodTestResult returns the result of processing a single pod built by [newTerminatedPodTestPod].
// The version hash is the custom resource version that the pod transformer computes from the pod model.
func newTerminatedPodTestResult(t *testing.T, pod *corev1.Pod, status, versionHash string) *collectors.CollectorRunResult {
	manifestPod := pod.DeepCopy()
	manifestPod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
	content, err := json.Marshal(manifestPod)
	require.NoError(t, err)

	podTags := []string{"pod_status:" + strings.ToLower(status)}

	metadata := &model.CollectorPod{
		ClusterName: "test-cluster",
		ClusterId:   "test-cluster",
		GroupId:     1,
		GroupSize:   1,
		Pods: []*model.Pod{
			{
				Metadata: &model.Metadata{
					Name:              pod.Name,
					Namespace:         pod.Namespace,
					Uid:               string(pod.UID),
					CreationTimestamp: pod.CreationTimestamp.Unix(),
					DeletionTimestamp: pod.DeletionTimestamp.Unix(),
					Labels:            []string{"app:my-app"},
					Annotations:       []string{"annotation:my-annotation"},
					ResourceVersion:   versionHash,
				},
				NodeName: pod.Spec.NodeName,
				Phase:    string(pod.Status.Phase),
				Status:   status,
				Yaml:     content,
				Tags:     podTags,
				ResourceRequirements: []*model.ResourceRequirements{
					{
						Limits:   map[string]int64{},
						Name:     "nginx",
						Requests: map[string]int64{},
						Type:     model.ResourceRequirementsType_container,
					},
				},
			},
		},
		Tags: []string{"kube_api_version:v1"},
	}

	manifest := &model.CollectorManifest{
		ClusterName: "test-cluster",
		ClusterId:   "test-cluster",
		GroupId:     1,
		GroupSize:   1,
		Manifests: []*model.Manifest{
			{
				Type:            int32(orchestrator.K8sPod),
				ResourceVersion: versionHash,
				Uid:             string(pod.UID),
				Content:         content,
				ContentType:     "json",
				Version:         "v1",
				Tags:            append([]string{"kube_api_version:v1"}, podTags...),
				ApiVersion:      "v1",
				NodeName:        pod.Spec.NodeName,
				Kind:            "Pod",
			},
		},
		OriginCollector: model.OriginCollector_datadogAgent,
	}

	return &collectors.CollectorRunResult{
		Result: processors.ProcessResult{
			MetadataMessages: []model.MessageBody{metadata},
			ManifestMessages: []model.MessageBody{manifest},
		},
		ResourcesListed:    1,
		ResourcesProcessed: 1,
	}
}
