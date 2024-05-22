// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && test

// Package testing provides various helper functions and fixtures for use in testing.
package testing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	tmock "github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
)

var (
	// CommonTags are the list of expected tags that match the input from the included `pods.json` fixture
	CommonTags = map[string][]string{
		"kubernetes_pod_uid://c2319815-10d0-11e8-bd5a-42010af00137": {"pod_name:datadog-agent-jbm2k"},
		"kubernetes_pod_uid://2edfd4d9-10ce-11e8-bd5a-42010af00137": {"pod_name:fluentd-gcp-v2.0.10-9q9t4"},
		"kubernetes_pod_uid://2fdfd4d9-10ce-11e8-bd5a-42010af00137": {"pod_name:fluentd-gcp-v2.0.10-p13r3"},
		"container_id://5741ed2471c0e458b6b95db40ba05d1a5ee168256638a0264f08703e48d76561": {
			"kube_container_name:fluentd-gcp",
			"kube_deployment:fluentd-gcp-v2.0.10",
			"kube_namespace:default",
		},
		"container_id://1d1f139dc1c9d49010512744df34740abcfaadf9930d3afd85afbf5fccfadbd6": {
			"kube_container_name:init",
			"kube_deployment:fluentd-gcp-v2.0.10",
			"kube_namespace:default",
		},
		"container_id://580cb469826a10317fd63cc780441920f49913ae63918d4c7b19a72347645b05": {
			"kube_container_name:prometheus-to-sd-exporter",
			"kube_deployment:fluentd-gcp-v2.0.10",
			"kube_namespace:default",
		},
		"container_id://6941ed2471c0e458b6b95db40ba05d1a5ee168256638a0264f08703e48d76561": {
			"kube_container_name:fluentd-gcp",
			"kube_deployment:fluentd-gcp-v2.0.10",
			"kube_namespace:default",
		},
		"container_id://690cb469826a10317fd63cc780441920f49913ae63918d4c7b19a72347645b05": {
			"kube_container_name:prometheus-to-sd-exporter",
			"kube_deployment:fluentd-gcp-v2.0.10",
			"kube_namespace:default",
		},
		"container_id://5f93d91c7aee0230f77fbe9ec642dd60958f5098e76de270a933285c24dfdc6f": {
			"pod_name:demo-app-success-c485bc67b-klj45",
			"kube_namespace:default",
		},
		"container_id://580cb469826a10317fd63cc780441920f49913ae63918d4c7b19a72347645b06": {
			"kube_container_name:prometheus-to-sd-exporter-no-namespace",
			"kube_deployment:fluentd-gcp-v2.0.10",
		},
		"container_id://6941ed2471c0e458b6b95db40ba05d1a5ee168256638a0264f08703e48d76562": {
			"kube_container_name:fluentd-gcp-no-namespace",
			"kube_deployment:fluentd-gcp-v2.0.10",
		},
		"container_id://690cb469826a10317fd63cc780441920f49913ae63918d4c7b19a72347645b06": {
			"kube_container_name:prometheus-to-sd-exporter-no-namespace",
			"kube_deployment:fluentd-gcp-v2.0.10",
		},
		"container_id://5f93d91c7aee0230f77fbe9ec642dd60958f5098e76de270a933285c24dfdc6g": {
			"pod_name:demo-app-success-c485bc67b-klj45-no-namespace",
		},
		"kubernetes_pod_uid://d2e71e36-10d0-11e8-bd5a-42010af00137": {"pod_name:dd-agent-q6hpw"},
		"kubernetes_pod_uid://260c2b1d43b094af6d6b4ccba082c2db": {
			"pod_name:kube-proxy-gke-haissam-default-pool-be5066f1-wnvn",
		},
		"kubernetes_pod_uid://24d6daa3-10d8-11e8-bd5a-42010af00137":                       {"pod_name:demo-app-success-c485bc67b-klj45"},
		"container_id://f69aa93ce78ee11e78e7c75dc71f535567961740a308422dafebdb4030b04903": {"pod_name:pi-kff76"},
		"kubernetes_pod_uid://12ceeaa9-33ca-11e6-ac8f-42010af00003":                       {"pod_name:dd-agent-ntepl"},
		"container_id://32fc50ecfe24df055f6d56037acb966337eef7282ad5c203a1be58f2dd2fe743": {"pod_name:dd-agent-ntepl", "kube_namespace:default"},
		"container_id://a335589109ce5506aa69ba7481fc3e6c943abd23c5277016c92dac15d0f40479": {
			"kube_container_name:datadog-agent",
			"kube_namespace:default",
		},
		"container_id://80bd9ebe296615341c68d571e843d800fb4a75bef696d858065572ab4e49920b": {
			"kube_container_name:running-init",
			"kube_namespace:default",
		},
		"container_id://326b384481ca95204018e3e837c61e522b64a3b86c3804142a22b2d1db9dbd7b": {
			"kube_container_name:datadog-agent",
			"kube_namespace:default",
		},
		"container_id://6d8c6a05731b52195998c438fdca271b967b171f6c894f11ba59aa2f4deff10c": {"pod_name:cassandra-0", "kube_namespace:default"},
		"kubernetes_pod_uid://639980e5-2e6c-11ea-8bb1-42010a800074": {
			"kube_namespace:default",
			"kube_service:nginx",
			"kube_stateful_set:web",
			"namespace:default",
			"persistentvolumeclaim:www-web-2",
			"pod_phase:running",
		},
		"kubernetes_pod_uid://639980e5-2e6c-11ea-8bb1-42010a800075": {
			"kube_namespace:default",
			"kube_service:nginx",
			"kube_stateful_set:web",
			"namespace:default",
			"persistentvolumeclaim:www-web-2",
			"persistentvolumeclaim:www2-web-3",
			"pod_phase:running",
		},
	}

	// InstanceTags are default tags that should be applied to all metrics at the instance level
	InstanceTags = []string{"instance_tag:something"}
)

// EndpointResponse represents a mock response to a kubelet endpoint, as well as where it should be populated from.
type EndpointResponse struct {
	filename string
	code     int
	err      error
}

// NewEndpointResponse creates a new EndpointResponse.
func NewEndpointResponse(filename string, code int, err error) EndpointResponse {
	return EndpointResponse{
		filename: filename,
		code:     code,
		err:      err,
	}
}

// CreateKubeletMock creates a new mock.KubeletMock with a given response for a given endpoint.
func CreateKubeletMock(response EndpointResponse, endpoint string) (*mock.KubeletMock, error) {
	var err error

	kubeletMock := mock.NewKubeletMock()
	var content []byte
	if response.filename != "" {
		content, err = os.ReadFile(response.filename)
		if err != nil {
			return nil, fmt.Errorf(fmt.Sprintf("unable to read test file at: %s, Err: %v", response.filename, err))
		}
	}
	kubeletMock.MockReplies[endpoint] = &mock.HTTPReplyMock{
		Data:         content,
		ResponseCode: response.code,
		Error:        response.err,
	}
	return kubeletMock, nil
}

// StorePopulatedFromFile populates a workloadmeta.Store based on pod data from a given file.
func StorePopulatedFromFile(store workloadmeta.Mock, filename string, podUtils *common.PodUtils) error {
	if filename == "" {
		return nil
	}

	podList, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("unable to load pod list, Err: %v", err))
	}
	var pods *kubelet.PodList
	err = json.Unmarshal(podList, &pods)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("unable to load pod list, Err: %v", err))
	}

	for _, pod := range pods.Items {
		podContainers := make([]workloadmeta.OrchestratorContainer, 0, len(pod.Status.Containers))

		for _, container := range pod.Status.Containers {
			if container.ID == "" {
				// A container without an ID has not been created by
				// the runtime yet, so we ignore them until it's
				// detected again.
				continue
			}

			image, err := workloadmeta.NewContainerImage(container.ImageID, container.Image)
			if err != nil {
				if errors.Is(err, containers.ErrImageIsSha256) {
					// try the resolved image ID if the image name in the container
					// status is a SHA256. this seems to happen sometimes when
					// pinning the image to a SHA256
					image, _ = workloadmeta.NewContainerImage(container.ImageID, container.ImageID)
				}
			}

			_, containerID := containers.SplitEntityName(container.ID)
			podContainer := workloadmeta.OrchestratorContainer{
				ID:   containerID,
				Name: container.Name,
			}
			podContainer.Image, _ = workloadmeta.NewContainerImage(container.ImageID, container.Image)

			podContainer.Image.ID = container.ImageID

			podContainers = append(podContainers, podContainer)
			store.Set(&workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   containerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: container.Name,
					Labels: map[string]string{
						kubernetes.CriContainerNamespaceLabel: pod.Metadata.Namespace,
					},
				},
				Image: image,
			})
		}

		store.Set(&workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   pod.Metadata.UID,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:        pod.Metadata.Name,
				Namespace:   pod.Metadata.Namespace,
				Annotations: pod.Metadata.Annotations,
				Labels:      pod.Metadata.Labels,
			},
			Containers: podContainers,
		})
		podUtils.PopulateForPod(pod)
	}
	return err
}

// AssertMetricCallsMatch is a helper function which allows us to assert that, for a given test and a given set of expected
// metrics, ONLY the expected metrics have been called, and ALL the expected metrics have been called.
func AssertMetricCallsMatch(t *testing.T, expectedMetrics []string, mockSender *mocksender.MockSender) {
	// note: this is awful and ugly, but it works for now
	var matchedAsserts []tmock.Call
	// Make sure that every metric in the expectedMetrics slice has been called
	for _, expectedMetric := range expectedMetrics {
		matches := 0
		for _, call := range mockSender.Calls {
			expected := tmock.Arguments{expectedMetric, tmock.AnythingOfType("float64"), "", mocksender.MatchTagsContains(InstanceTags)}
			if _, diffs := expected.Diff(call.Arguments); diffs == 0 {
				matches++
				matchedAsserts = append(matchedAsserts, call)
			}
		}
		if matches == 0 {
			t.Errorf("expected metric %s to be called, but it was not", expectedMetric)
		}
	}

	// find out output any actual calls which exist which were not in the expected list
	if len(matchedAsserts) != len(mockSender.Calls) {
		var calledWithArgs []string
		for _, call := range mockSender.Calls {
			wasMatched := false
			for _, matched := range matchedAsserts {
				if call.Method == matched.Method {
					if _, diffs := matched.Arguments.Diff(call.Arguments); diffs == 0 {
						wasMatched = true
						break
					}
				}
			}
			if !wasMatched {
				calledWithArgs = append(calledWithArgs, fmt.Sprintf("%v", call.Arguments))
			}
		}
		t.Errorf("expected %v metrics to be matched, but %v were", len(mockSender.Calls), len(matchedAsserts))
		t.Errorf("missing assertions for calls:\n        %v", strings.Join(calledWithArgs, "\n"))
	}
}
