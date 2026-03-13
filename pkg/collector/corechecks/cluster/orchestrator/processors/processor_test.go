// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package processors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// ProcessorTestSuite is a test suite for the Processor.
type ProcessorTestSuite struct {
	suite.Suite
}

func (s *ProcessorTestSuite) SetupTest() {
	orchestrator.KubernetesResourceCache = orchestrator.NewKubernetesResourceCache()
}

type testProcessScenario struct {
	inputHandlers          Handlers
	inputProcessorContext  *processorstest.ProcessorContext
	inputResource          *processorstest.Resource
	outputListed           int
	outputProcessed        int
	outputResourceMetadata *processorstest.Resource
	outputResourceManifest *processorstest.Resource
}

func (s *ProcessorTestSuite) testProcessScenario(scenario testProcessScenario) {
	processor := Processor{h: scenario.inputHandlers}
	resource := scenario.inputResource
	expectedResourceMetadata := scenario.outputResourceMetadata
	expectedResourceManifest := scenario.outputResourceManifest

	expectedResult := func() ProcessResult {
		result := ProcessResult{
			MetadataMessages: []model.MessageBody{},
			ManifestMessages: []model.MessageBody{},
		}

		if scenario.outputProcessed <= 0 {
			return result
		}

		result.MetadataMessages = []model.MessageBody{
			&model.CollectorManifest{
				AgentVersion: scenario.inputProcessorContext.GetAgentVersion(),
				ClusterName:  scenario.inputProcessorContext.GetOrchestratorConfig().KubeClusterName,
				ClusterId:    scenario.inputProcessorContext.GetClusterID(),
				GroupId:      scenario.inputProcessorContext.GetMsgGroupID(),
				GroupSize:    1,
				Manifests: []*model.Manifest{
					{
						Content: processorstest.MustMarshalJSON(expectedResourceMetadata),
					},
				},
			},
		}

		if scenario.inputProcessorContext.IsManifestProducer() {
			result.ManifestMessages = []model.MessageBody{
				&model.CollectorManifest{
					AgentVersion: scenario.inputProcessorContext.GetAgentVersion(),
					ClusterName:  scenario.inputProcessorContext.GetOrchestratorConfig().KubeClusterName,
					ClusterId:    scenario.inputProcessorContext.GetClusterID(),
					GroupId:      scenario.inputProcessorContext.GetMsgGroupID(),
					GroupSize:    1,
					HostName:     scenario.inputProcessorContext.GetHostName(),
					SystemInfo:   scenario.inputProcessorContext.GetSystemInfo(),
					Manifests: []*model.Manifest{
						{
							ApiVersion:      "apiGroup/v1",
							Kind:            "ResourceKind",
							Content:         processorstest.MustMarshalJSON(expectedResourceManifest),
							ContentType:     "json",
							IsTerminated:    scenario.inputProcessorContext.IsTerminatedResources(),
							NodeName:        "node",
							ResourceVersion: expectedResourceManifest.ResourceVersion,
							Tags:            []string{"collector_tag:collector_tag_value", "metadata_tag:metadata_tag_value"},
							Type:            1,
							Uid:             expectedResourceManifest.ResourceUID,
							Version:         "v1",
						},
					},
				},
			}
		}

		return result
	}

	result, listed, processed := processor.Process(scenario.inputProcessorContext, []*processorstest.Resource{resource})
	s.Require().Equal(scenario.outputListed, listed)
	s.Require().Equal(scenario.outputProcessed, processed)
	s.Equal(expectedResult(), result)
}

func (s *ProcessorTestSuite) TestProcess() {
	s.testProcessScenario(testProcessScenario{
		inputHandlers:          &TestResourceHandlers{},
		inputProcessorContext:  processorstest.NewProcessorContext(),
		inputResource:          processorstest.NewResource(),
		outputListed:           1,
		outputProcessed:        1,
		outputResourceManifest: processorstest.NewExpectedResourceManifest(),
		outputResourceMetadata: processorstest.NewExpectedResourceMetadata(),
	})
}

func (s *ProcessorTestSuite) TestProcess_Panic() {
	s.testProcessScenario(testProcessScenario{
		inputHandlers:         &TestResourceHandlers{PanicInResourceList: true},
		inputProcessorContext: processorstest.NewProcessorContext(),
		inputResource:         processorstest.NewResource(),
		outputProcessed:       -1,
	})
}

func (s *ProcessorTestSuite) TestProcess_TerminatedResources_DeletionTimestampMissing() {
	processorContext := processorstest.NewProcessorContext()
	processorContext.TerminatedResources = true

	deletionTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	processorContext.Clock.Set(deletionTime)

	outputResourceMetadata := processorstest.NewExpectedResourceMetadata()
	outputResourceMetadata.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: deletionTime}

	outputResourceManifest := processorstest.NewExpectedResourceManifest()
	outputResourceManifest.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: deletionTime}

	s.testProcessScenario(testProcessScenario{
		inputHandlers:          &TestResourceHandlers{},
		inputProcessorContext:  processorContext,
		inputResource:          processorstest.NewResource(),
		outputListed:           1,
		outputProcessed:        1,
		outputResourceMetadata: outputResourceMetadata,
		outputResourceManifest: outputResourceManifest,
	})
}

func (s *ProcessorTestSuite) TestProcess_TerminatedResources_DeletionTimestampPresent() {
	processorContext := processorstest.NewProcessorContext()
	processorContext.TerminatedResources = true

	resource := processorstest.NewResource()
	resource.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	expectedResourceMetadata := processorstest.NewExpectedResourceMetadata()
	expectedResourceMetadata.ObjectMeta.DeletionTimestamp = resource.ObjectMeta.DeletionTimestamp

	expectedResourceManifest := processorstest.NewExpectedResourceManifest()
	expectedResourceManifest.ObjectMeta.DeletionTimestamp = resource.ObjectMeta.DeletionTimestamp

	s.testProcessScenario(testProcessScenario{
		inputHandlers:          &TestResourceHandlers{},
		inputProcessorContext:  processorContext,
		inputResource:          resource,
		outputListed:           1,
		outputProcessed:        1,
		outputResourceMetadata: expectedResourceMetadata,
		outputResourceManifest: expectedResourceManifest,
	})
}

func (s *ProcessorTestSuite) TestProcess_NotManifestProducer() {
	processorContext := processorstest.NewProcessorContext()
	processorContext.ManifestProducer = false
	s.testProcessScenario(testProcessScenario{
		inputHandlers:          &TestResourceHandlers{},
		inputProcessorContext:  processorContext,
		inputResource:          processorstest.NewResource(),
		outputListed:           1,
		outputProcessed:        1,
		outputResourceMetadata: processorstest.NewExpectedResourceMetadata(),
	})
}

func (s *ProcessorTestSuite) TestProcess_SkipCacheHit() {
	resource := processorstest.NewResource()
	orchestrator.KubernetesResourceCache.Set(resource.ResourceUID, resource.ResourceVersion, 0)
	s.testProcessScenario(testProcessScenario{
		inputHandlers:         &TestResourceHandlers{},
		inputProcessorContext: processorstest.NewProcessorContext(),
		inputResource:         resource,
		outputListed:          1,
		outputProcessed:       0,
	})
}

func (s *ProcessorTestSuite) TestProcess_SkipBeforeCacheCheck() {
	s.testProcessScenario(testProcessScenario{
		inputHandlers:         &TestResourceHandlers{SkipBeforeCacheCheck: true},
		inputProcessorContext: processorstest.NewProcessorContext(),
		inputResource:         processorstest.NewResource(),
		outputListed:          1,
		outputProcessed:       0,
	})
}

func (s *ProcessorTestSuite) TestProcess_SkipAfterMarshalling_ManifestCollectionDisabled() {
	processorContext := processorstest.NewProcessorContext()
	processorContext.GetOrchestratorConfig().IsManifestCollectionEnabled = false
	s.testProcessScenario(testProcessScenario{
		inputHandlers:         &TestResourceHandlers{SkipAfterMarshalling: true},
		inputProcessorContext: processorContext,
		inputResource:         processorstest.NewResource(),
		outputListed:          1,
		outputProcessed:       0,
	})
}

func (s *ProcessorTestSuite) TestProcess_SkipAfterMarshalling_ManifestCollectionEnabled() {
	s.testProcessScenario(testProcessScenario{
		inputHandlers:          &TestResourceHandlers{},
		inputProcessorContext:  processorstest.NewProcessorContext(),
		inputResource:          processorstest.NewResource(),
		outputListed:           1,
		outputProcessed:        1,
		outputResourceManifest: processorstest.NewExpectedResourceManifest(),
		outputResourceMetadata: processorstest.NewExpectedResourceMetadata(),
	})
}

func (s *ProcessorTestSuite) TestProcess_SkipBeforeMarshalling() {
	s.testProcessScenario(testProcessScenario{
		inputHandlers:         &TestResourceHandlers{SkipBeforeMarshalling: true},
		inputProcessorContext: processorstest.NewProcessorContext(),
		inputResource:         processorstest.NewResource(),
		outputListed:          1,
		outputProcessed:       0,
	})
}

func TestProcessorTestSuite(t *testing.T) {
	suite.Run(t, new(ProcessorTestSuite))
}

func TestSortedMarshal(t *testing.T) {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Annotations: map[string]string{
				"b-annotation":   "test",
				"ab-annotation":  "test",
				"a-annotation":   "test",
				"ac-annotation":  "test",
				"ba-annotation":  "test",
				"1ab-annotation": "test",
			},
		},
	}
	json, err := json.Marshal(p)
	assert.NoError(t, err)

	//nolint:revive // TODO(CAPP) Fix revive linter
	expectedJson := `{
						"metadata":{
							"name":"test-pod",
							"creationTimestamp":null,
							"annotations":{
								"1ab-annotation":"test",
								"a-annotation":"test",
								"ab-annotation":"test",
								"ac-annotation":"test",
								"b-annotation":"test",
								"ba-annotation":"test"
							}
						},
						"spec":{
							"containers":null
						},
						"status":{}
					}`
	//nolint:revive // TODO(CAPP) Fix revive linter
	actualJson := string(json)
	assert.JSONEq(t, expectedJson, actualJson)
}

func TestInsertDeletionTimestampIfPossible(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		obj      interface{}
		expected interface{}
	}{
		{
			name:     "nil object",
			obj:      nil,
			expected: nil,
		},
		{
			name:     "non-pointer type",
			obj:      appsv1.ReplicaSet{},
			expected: appsv1.ReplicaSet{},
		},
		{
			name:     "non-struct type",
			obj:      &[]string{},
			expected: &[]string{},
		},
		{
			name: "object without ObjectMeta",
			obj: &struct {
				Name string
			}{Name: "test"},
			expected: &struct {
				Name string
			}{Name: "test"},
		},
		{
			name: "object with existing DeletionTimestamp",
			obj: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rs",
					DeletionTimestamp: &metav1.Time{Time: now.Add(-time.Hour)},
				},
			},
			expected: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rs",
					DeletionTimestamp: &metav1.Time{Time: now.Add(-time.Hour)},
				},
			},
		},
		{
			name: "unstructured object",
			obj:  &unstructured.Unstructured{},
			expected: func() interface{} {
				u := &unstructured.Unstructured{}
				u.SetDeletionTimestamp(&metav1.Time{Time: now})
				return u
			}(),
		},
		{
			name: "regular kubernetes object",
			obj: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rs",
				},
			},
			expected: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rs",
					DeletionTimestamp: &metav1.Time{Time: now},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insertDeletionTimestampIfPossible(tt.obj, now)
			require.Equal(t, tt.expected, result)
		})
	}
}
