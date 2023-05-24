// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy

package sbom

import (
	"os"
	"testing"
	"time"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	model "github.com/DataDog/agent-payload/v5/sbom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	fakeworkloadmeta "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"
)

func TestProcessEvents(t *testing.T) {
	sbomGenerationTime := time.Now()

	tests := []struct {
		name          string
		inputEvents   []workloadmeta.Event
		expectedSBOMs []*model.SBOMEntity
	}{
		{
			name: "standard case",
			inputEvents: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.ContainerImageMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainerImageMetadata,
							ID:   "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						},
						RepoTags: []string{
							"datadog/agent:7-rc",
							"datadog/agent:7.41.1-rc.1",
							"gcr.io/datadoghq/agent:7-rc",
							"gcr.io/datadoghq/agent:7.41.1-rc.1",
							"public.ecr.aws/datadog/agent:7-rc",
							"public.ecr.aws/datadog/agent:7.41.1-rc.1",
						},
						RepoDigests: []string{
							"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
							"gcr.io/datadoghq/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
							"public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
						},
						SBOM: &workloadmeta.SBOM{
							CycloneDXBOM: &cyclonedx.BOM{
								SpecVersion: cyclonedx.SpecVersion1_4,
								Version:     42,
								Components: &[]cyclonedx.Component{
									{
										Name: "Foo",
									},
									{
										Name: "Bar",
									},
									{
										Name: "Baz",
									},
								},
							},
							GenerationTime:     sbomGenerationTime,
							GenerationDuration: 10 * time.Second,
						},
					},
				},
			},
			expectedSBOMs: []*model.SBOMEntity{
				{
					Type: model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:   "datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:datadog/agent",
						"short_image:agent",
						"image_tag:7-rc",
						"image_tag:7.41.1-rc.1",
					},
					RepoTags: []string{
						"7-rc",
						"7.41.1-rc.1",
					},
					InUse:              false,
					GeneratedAt:        timestamppb.New(sbomGenerationTime),
					GenerationDuration: durationpb.New(10 * time.Second),
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
							Components: []*cyclonedx_v1_4.Component{
								{
									Name: "Foo",
								},
								{
									Name: "Bar",
								},
								{
									Name: "Baz",
								},
							},
						},
					},
				},
				{
					Type: model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:   "gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:gcr.io/datadoghq/agent",
						"short_image:agent",
						"image_tag:7-rc",
						"image_tag:7.41.1-rc.1",
					},
					RepoTags: []string{
						"7-rc",
						"7.41.1-rc.1",
					},
					InUse:              false,
					GeneratedAt:        timestamppb.New(sbomGenerationTime),
					GenerationDuration: durationpb.New(10 * time.Second),
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
							Components: []*cyclonedx_v1_4.Component{
								{
									Name: "Foo",
								},
								{
									Name: "Bar",
								},
								{
									Name: "Baz",
								},
							},
						},
					},
				},
				{
					Type: model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:   "public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:public.ecr.aws/datadog/agent",
						"short_image:agent",
						"image_tag:7-rc",
						"image_tag:7.41.1-rc.1",
					},
					RepoTags: []string{
						"7-rc",
						"7.41.1-rc.1",
					},
					InUse:              false,
					GeneratedAt:        timestamppb.New(sbomGenerationTime),
					GenerationDuration: durationpb.New(10 * time.Second),
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
							Components: []*cyclonedx_v1_4.Component{
								{
									Name: "Foo",
								},
								{
									Name: "Bar",
								},
								{
									Name: "Baz",
								},
							},
						},
					},
				},
			},
		},
		{
			// In containerd some images are created without a repo digest, and it's
			// also possible to remove repo digests manually. To test that scenario, in
			// this test, we define an image with 2 repo tags: one for the gcr.io
			// registry and another for the public.ecr.aws registry, but there's only
			// one repo digest.
			// We expect to send 2 events, one for each registry.
			name: "repo tag with no matching repo digest",
			inputEvents: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.ContainerImageMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainerImageMetadata,
							ID:   "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						},
						RepoTags: []string{
							"gcr.io/datadoghq/agent:7-rc",
							"public.ecr.aws/datadog/agent:7-rc",
						},
						RepoDigests: []string{
							// Notice that there's a repo tag for gcr.io, but no repo digest.
							"public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
						},
						SBOM: &workloadmeta.SBOM{
							CycloneDXBOM: &cyclonedx.BOM{
								SpecVersion: cyclonedx.SpecVersion1_4,
								Version:     42,
								Components: &[]cyclonedx.Component{
									{
										Name: "Foo",
									},
									{
										Name: "Bar",
									},
									{
										Name: "Baz",
									},
								},
							},
							GenerationTime:     sbomGenerationTime,
							GenerationDuration: 10 * time.Second,
						},
					},
				},
			},
			expectedSBOMs: []*model.SBOMEntity{
				{
					Type: model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:   "public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:public.ecr.aws/datadog/agent",
						"short_image:agent",
						"image_tag:7-rc",
					},
					RepoTags: []string{
						"7-rc",
					},
					InUse:              false,
					GeneratedAt:        timestamppb.New(sbomGenerationTime),
					GenerationDuration: durationpb.New(10 * time.Second),
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
							Components: []*cyclonedx_v1_4.Component{
								{
									Name: "Foo",
								},
								{
									Name: "Bar",
								},
								{
									Name: "Baz",
								},
							},
						},
					},
				},
				{
					Type: model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:   "gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:gcr.io/datadoghq/agent",
						"short_image:agent",
						"image_tag:7-rc",
					},
					RepoTags: []string{
						"7-rc",
					},
					InUse:              false,
					GeneratedAt:        timestamppb.New(sbomGenerationTime),
					GenerationDuration: durationpb.New(10 * time.Second),
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
							Components: []*cyclonedx_v1_4.Component{
								{
									Name: "Foo",
								},
								{
									Name: "Bar",
								},
								{
									Name: "Baz",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Validate InUse flag",
			inputEvents: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.ContainerImageMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainerImageMetadata,
							ID:   "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						},
						RepoTags:    []string{"datadog/agent:7-rc"},
						RepoDigests: []string{"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409"},
						SBOM: &workloadmeta.SBOM{
							CycloneDXBOM: &cyclonedx.BOM{
								SpecVersion: cyclonedx.SpecVersion1_4,
								Version:     42,
							},
							GenerationTime:     sbomGenerationTime,
							GenerationDuration: 10 * time.Second,
						},
					},
				},
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainer,
							ID:   "fb9843d6f3e4d9506b08f5ddada74262b7ebf1cf60edb49c71d6c856fd43b75a",
						},
						Image: workloadmeta.ContainerImage{
							ID: "datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
						},
						State: workloadmeta.ContainerState{
							Running: true,
						},
					},
				},
			},
			expectedSBOMs: []*model.SBOMEntity{
				{
					Type: model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:   "datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:datadog/agent",
						"short_image:agent",
						"image_tag:7-rc",
					},
					RepoTags: []string{
						"7-rc",
					},
					InUse:              false,
					GeneratedAt:        timestamppb.New(sbomGenerationTime),
					GenerationDuration: durationpb.New(10 * time.Second),
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
						},
					},
				},
				{
					Type: model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:   "datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:datadog/agent",
						"short_image:agent",
						"image_tag:7-rc",
					},
					RepoTags: []string{
						"7-rc",
					},
					InUse:              true,
					GeneratedAt:        timestamppb.New(sbomGenerationTime),
					GenerationDuration: durationpb.New(10 * time.Second),
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
						},
					},
				},
			},
		},
	}

	cfg := config.Mock(nil)
	cacheDir, err := os.MkdirTemp("", "sbom-cache")
	assert.Nil(t, err)
	defer os.RemoveAll(cacheDir)
	cfg.Set("sbom.cache_directory", cacheDir)
	cfg.Set("container_image_collection.sbom.enabled", true)
	_, err = sbomscanner.CreateGlobalScanner(cfg)
	assert.Nil(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var SBOMsSent = atomic.NewInt32(0)

			fakeworkloadmeta := fakeworkloadmeta.NewStore()

			sender := mocksender.NewMockSender("")
			sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Run(func(_ mock.Arguments) {
				SBOMsSent.Inc()
			})

			// Define a max size of 1 for the queue. With a size > 1, it's difficult to
			// control the number of events sent on each call.
			p, err := newProcessor(fakeworkloadmeta, sender, 1, 50*time.Millisecond, false)
			if err != nil {
				t.Fatal(err)
			}

			for _, ev := range test.inputEvents {
				switch ev.Type {
				case workloadmeta.EventTypeSet:
					fakeworkloadmeta.Set(ev.Entity)
				case workloadmeta.EventTypeUnset:
					fakeworkloadmeta.Unset(ev.Entity)
				}
			}

			p.processContainerImagesEvents(workloadmeta.EventBundle{
				Events: test.inputEvents,
				Ch:     make(chan struct{}),
			})

			p.stop()

			// The queue is processing the events in a different go routine and might
			// need some time
			assert.Eventually(t, func() bool {
				return SBOMsSent.Load() == int32(len(test.expectedSBOMs))
			}, 1*time.Second, 5*time.Millisecond)

			for _, expectedSBOM := range test.expectedSBOMs {
				encoded, err := proto.Marshal(&model.SBOMPayload{
					Version:  1,
					Source:   &sourceAgent,
					Entities: []*model.SBOMEntity{expectedSBOM},
					DdEnv:    &envVarEnv,
				})
				assert.Nil(t, err)
				sender.AssertEventPlatformEvent(t, encoded, epforwarder.EventTypeContainerSBOM)
			}
		})
	}
}
