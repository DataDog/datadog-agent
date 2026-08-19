// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || windows

package sbom

import (
	"context"
	"testing"
	"time"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	model "github.com/DataDog/agent-payload/v5/sbom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/atomic"
	"go.uber.org/fx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DataDog/datadog-agent/comp/core"
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestProcessEvents(t *testing.T) {
	hname, _ := hostname.Get(context.TODO())
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
						SBOM: mustCompressSBOM(t, &workloadmeta.SBOM{
							CycloneDXBOM: &cyclonedx_v1_4.Bom{
								SpecVersion: cyclonedx.SpecVersion1_4.String(),
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
							GenerationTime:     sbomGenerationTime,
							GenerationDuration: 10 * time.Second,
							Status:             workloadmeta.Success,
						}),
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
					RepoDigests: []string{
						"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
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
					Status: model.SBOMStatus_SUCCESS,
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
					RepoDigests: []string{
						"gcr.io/datadoghq/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
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
					Status: model.SBOMStatus_SUCCESS,
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
					RepoDigests: []string{
						"public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
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
					Status: model.SBOMStatus_SUCCESS,
				},
			},
		},
		{
			// In containerd some images are created without a repo digest, and it's
			// also possible to remove repo digests manually. To test that scenario, in
			// this test, we define an image with 2 repo tags: one for the gcr.io
			// registry and another for the public.ecr.aws registry, but there's only
			// one repo digest.
			// We expect to send only one event as the backend-end will drop
			// sbom without any repo digest anyhow.
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
						SBOM: mustCompressSBOM(t, &workloadmeta.SBOM{
							CycloneDXBOM: &cyclonedx_v1_4.Bom{
								SpecVersion: cyclonedx.SpecVersion1_4.String(),
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
							GenerationTime:     sbomGenerationTime,
							GenerationDuration: 10 * time.Second,
							Status:             workloadmeta.Success,
						}),
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
					RepoDigests: []string{
						"public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
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
					Status: model.SBOMStatus_SUCCESS,
				},
			},
		},
		{
			name: "no repo digest",
			inputEvents: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.ContainerImageMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainerImageMetadata,
							ID:   "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name: "my-image:latest",
						},
						RepoTags: []string{
							"my-image:latest",
						},
						SBOM: mustCompressSBOM(t, &workloadmeta.SBOM{
							CycloneDXBOM: &cyclonedx_v1_4.Bom{
								SpecVersion: cyclonedx.SpecVersion1_4.String(),
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
							GenerationTime:     sbomGenerationTime,
							GenerationDuration: 10 * time.Second,
							Status:             workloadmeta.Success,
						}),
					},
				},
			},
			expectedSBOMs: []*model.SBOMEntity{
				{
					Type: model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:   "my-image@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:my-image@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:my-image",
						"short_image:my-image",
						"image_tag:latest",
					},
					RepoTags: []string{
						"latest",
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
					Status: model.SBOMStatus_SUCCESS,
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
						SBOM: mustCompressSBOM(t, &workloadmeta.SBOM{
							CycloneDXBOM: &cyclonedx_v1_4.Bom{
								SpecVersion: cyclonedx.SpecVersion1_4.String(),
								Version:     pointer.Ptr(int32(42)),
							},
							GenerationTime:     sbomGenerationTime,
							GenerationDuration: 10 * time.Second,
							Status:             workloadmeta.Success,
						}),
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
			// A single SBOM is emitted: the store already knows about the
			// running container when the image event is processed.
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
					RepoDigests: []string{
						"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
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
					Status: model.SBOMStatus_SUCCESS,
				},
			},
		},
		{
			name: "pending case",
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
						SBOM: mustCompressSBOM(t, &workloadmeta.SBOM{
							Status: workloadmeta.Pending,
						}),
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
					RepoDigests: []string{
						"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
					},
					InUse:  false,
					Status: model.SBOMStatus_PENDING,
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
					RepoDigests: []string{
						"gcr.io/datadoghq/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
					},
					InUse:  false,
					Status: model.SBOMStatus_PENDING,
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
					RepoDigests: []string{
						"public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
					},
					InUse:  false,
					Status: model.SBOMStatus_PENDING,
				},
			},
		},
		{
			name: "error case",
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
						SBOM: mustCompressSBOM(t, &workloadmeta.SBOM{
							Status: workloadmeta.Failed,
							Error:  "error",
						}),
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
					RepoDigests: []string{
						"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
					},
					InUse: false,
					Sbom: &model.SBOMEntity_Error{
						Error: "error",
					},
					Status: model.SBOMStatus_FAILED,
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
					RepoDigests: []string{
						"gcr.io/datadoghq/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
					},
					InUse: false,
					Sbom: &model.SBOMEntity_Error{
						Error: "error",
					},
					Status: model.SBOMStatus_FAILED,
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
					RepoDigests: []string{
						"public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
					},
					InUse: false,
					Sbom: &model.SBOMEntity_Error{
						Error: "error",
					},
					Status: model.SBOMStatus_FAILED,
				},
			},
		},
	}

	cacheDir := t.TempDir()

	cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{
		"sbom.cache_directory":                          cacheDir,
		"sbom.container_image.enabled":                  true,
		"sbom.container_image.allow_missing_repodigest": true,
	})
	wmeta := fxutil.Test[option.Option[workloadmeta.Component]](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	_, err := sbomscanner.CreateGlobalScanner(cfg, wmeta)
	assert.Nil(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			SBOMsSent := atomic.NewInt32(0)

			workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				fx.Provide(func() log.Component { return logmock.New(t) }),
				fx.Provide(func() configcomp.Component { return configcomp.NewMock(t) }),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			sender := mocksender.NewMockSender(t, "")
			sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Run(func(_ mock.Arguments) {
				SBOMsSent.Inc()
			})

			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)

			// Define a max size of 1 for the queue. With a size > 1, it's difficult to
			// control the number of events sent on each call.
			p, err := newProcessor(workloadmetaStore, mockFilterStore, sender, fakeTagger, cfg, 1, 50*time.Millisecond, time.Second)
			if err != nil {
				t.Fatal(err)
			}

			for _, ev := range test.inputEvents {
				switch ev.Type {
				case workloadmeta.EventTypeSet:
					workloadmetaStore.Set(ev.Entity)
				case workloadmeta.EventTypeUnset:
					workloadmetaStore.Unset(ev.Entity)
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

			envVarEnv := cfg.GetString("env")

			for _, expectedSBOM := range test.expectedSBOMs {
				encoded, err := proto.Marshal(&model.SBOMPayload{
					Version:  1,
					Host:     hname,
					Source:   &sourceAgent,
					Entities: []*model.SBOMEntity{expectedSBOM},
					DdEnv:    &envVarEnv,
				})
				assert.Nil(t, err)
				sender.AssertEventPlatformEvent(t, encoded, eventplatform.EventTypeContainerSBOM)
			}
		})
	}
}

// TestInUseFlagAccuracy covers scenarios that previously caused incorrect inUse values:
//  1. Same bundle: a container removal and an image SBOM update arrive together.
//     The SBOM should be emitted with inUse=false because the container was removed.
//  2. Container stopped (not yet removed): a container transitions to Running=false and
//     then a new SBOM update arrives. The SBOM should reflect inUse=false.
//  3. Containerd-style image ID: ctr.Image.ID is the raw image config digest rather than
//     a repo digest. The image should still be reported as inUse=true when a container runs it.
//  4. Image ID changing shape: a container is first described by the runtime alone, which
//     names its image by config digest, then also by the kubelet, which names it by repo
//     digest. Both spellings must stop being in use when the container stops.
//  5. Missed event: the container disappears from the store without the check being told.
//     The next SBOM must still report inUse=false.
//  6. Periodic refresh reporting inUse=false: a container starting the image again must
//     be reported, rather than taken for one that changes nothing.
func TestInUseFlagAccuracy(t *testing.T) {
	const (
		imageID     = "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd"
		repoDigest  = "datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409"
		containerID = "fb9843d6f3e4d9506b08f5ddada74262b7ebf1cf60edb49c71d6c856fd43b75a"
	)
	sbomTime := time.Now()

	// The SBOM version tells the payloads emitted for the initial image apart
	// from those emitted after it is rescanned, so that an assertion on the
	// second cannot be satisfied by the first.
	imageWithSBOMVersion := func(version int32) *workloadmeta.ContainerImageMetadata {
		return &workloadmeta.ContainerImageMetadata{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainerImageMetadata,
				ID:   imageID,
			},
			RepoTags:    []string{"datadog/agent:7-rc"},
			RepoDigests: []string{repoDigest},
			SBOM: mustCompressSBOM(t, &workloadmeta.SBOM{
				CycloneDXBOM: &cyclonedx_v1_4.Bom{
					SpecVersion: cyclonedx.SpecVersion1_4.String(),
					Version:     pointer.Ptr(version),
				},
				GenerationTime:     sbomTime,
				GenerationDuration: time.Second,
				Status:             workloadmeta.Success,
			}),
		}
	}

	imageEntity := imageWithSBOMVersion(1)
	rescannedImageEntity := imageWithSBOMVersion(2)

	runningContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: repoDigest},
		State:    workloadmeta.ContainerState{Running: true},
	}
	stoppedContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: repoDigest},
		State:    workloadmeta.ContainerState{Running: false},
	}

	makeExpectedSBOM := func(inUse bool, version int32) *model.SBOMEntity {
		return &model.SBOMEntity{
			Type:               model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
			Id:                 "datadog/agent@" + imageID,
			DdTags:             []string{"image_id:datadog/agent@" + imageID, "image_name:datadog/agent", "short_image:agent", "image_tag:7-rc"},
			RepoTags:           []string{"7-rc"},
			RepoDigests:        []string{repoDigest},
			InUse:              inUse,
			GeneratedAt:        timestamppb.New(sbomTime),
			GenerationDuration: durationpb.New(time.Second),
			Sbom: &model.SBOMEntity_Cyclonedx{Cyclonedx: &cyclonedx_v1_4.Bom{
				SpecVersion: "1.4",
				Version:     pointer.Ptr(version),
			}},
			Status: model.SBOMStatus_SUCCESS,
		}
	}

	cacheDir := t.TempDir()
	cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{
		"sbom.cache_directory":                          cacheDir,
		"sbom.container_image.enabled":                  true,
		"sbom.container_image.allow_missing_repodigest": true,
	})

	assertSBOMSent := func(t *testing.T, sender *mocksender.MockSender, entity *model.SBOMEntity) {
		t.Helper()

		hname, _ := hostname.Get(context.TODO())
		envVarEnv := cfg.GetString("env")
		encoded, err := proto.Marshal(&model.SBOMPayload{
			Version:  1,
			Host:     hname,
			Source:   &sourceAgent,
			Entities: []*model.SBOMEntity{entity},
			DdEnv:    &envVarEnv,
		})
		assert.Nil(t, err)
		sender.AssertEventPlatformEvent(t, encoded, eventplatform.EventTypeContainerSBOM)
	}

	if sbomscanner.GetGlobalScanner() == nil {
		wmeta := fxutil.Test[option.Option[workloadmeta.Component]](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))
		_, err := sbomscanner.CreateGlobalScanner(cfg, wmeta)
		assert.NoError(t, err)
	}

	makeBundle := func(events ...workloadmeta.Event) workloadmeta.EventBundle {
		return workloadmeta.EventBundle{Events: events, Ch: make(chan struct{})}
	}

	newTestSetup := func(t *testing.T) (*processor, *mocksender.MockSender, *atomic.Int32, workloadmetamock.Mock) {
		t.Helper()
		store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() configcomp.Component { return configcomp.NewMock(t) }),
			fx.Supply(context.Background()),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))
		counter := atomic.NewInt32(0)
		sender := mocksender.NewMockSender(t, "")
		sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Run(func(_ mock.Arguments) {
			counter.Inc()
		})
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)
		// Queue size 1 so each entity becomes its own event, matching the assertion style.
		p, err := newProcessor(store, mockFilterStore, sender, fakeTagger, cfg, 1, 50*time.Millisecond, time.Second)
		assert.Nil(t, err)
		return p, sender, counter, store
	}

	// Test 1: container removal and image SBOM update in the same bundle
	t.Run("container removed in the same bundle as the image SBOM", func(t *testing.T) {
		p, sender, counter, store := newTestSetup(t)

		// Establish initial state: running container.
		store.Set(imageEntity)
		store.Set(runningContainer)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: imageEntity},
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: runningContainer},
		))
		// Wait for the single SBOM from the setup phase (inUse=true).
		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)
		counter.Store(0)

		// The container is removed AND a fresh SBOM arrives in the same bundle.
		// The order the two events are handled in must not matter: the store
		// has already dropped the container by the time the bundle arrives.
		store.Unset(runningContainer)
		store.Set(rescannedImageEntity)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: rescannedImageEntity},
			workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: runningContainer},
		))
		p.stop()

		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)

		assertSBOMSent(t, sender, makeExpectedSBOM(false, 2))
	})

	// Test 2: container transitions stopped then image SBOM updates
	t.Run("stopped container does not keep inUse=true", func(t *testing.T) {
		p, sender, counter, store := newTestSetup(t)

		// Step 1: image + running container → one SBOM with inUse=true.
		store.Set(imageEntity)
		store.Set(runningContainer)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: imageEntity},
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: runningContainer},
		))
		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)
		counter.Store(0)

		// Step 2: container stops (Running=false) and a new image SBOM arrives.
		// The stopped container must no longer count as a user of the image.
		store.Set(stoppedContainer)
		store.Set(rescannedImageEntity)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: stoppedContainer},
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: rescannedImageEntity},
		))
		p.stop()

		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)

		assertSBOMSent(t, sender, makeExpectedSBOM(false, 2))
	})

	// Test 3: containerd-style image ID (raw sha256, not repo digest)
	t.Run("container names its image by config digest", func(t *testing.T) {
		p, sender, counter, store := newTestSetup(t)

		// Container uses the raw image config digest as its Image.ID (containerd style).
		containerdContainer := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
			Image:    workloadmeta.ContainerImage{ID: imageID}, // raw sha256, not repo digest
			State:    workloadmeta.ContainerState{Running: true},
		}

		store.Set(imageEntity)
		store.Set(containerdContainer)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: imageEntity},
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: containerdContainer},
		))
		p.stop()

		// The image is matched by its ID rather than by a repo digest.
		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)

		assertSBOMSent(t, sender, makeExpectedSBOM(true, 1))
	})

	// Test 4: on Kubernetes the merged container entity names its image by
	// config digest while the runtime is its only source, then by repo digest
	// once the kubelet describes it too. A check tracking container events
	// registers the container under both spellings but only ever removes it
	// from the last one, leaving the image in use forever.
	t.Run("image ID changing shape does not leave the image in use", func(t *testing.T) {
		p, sender, counter, store := newTestSetup(t)

		// The runtime is the only source: Image.ID is the config digest.
		runtimeOnly := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
			Image:    workloadmeta.ContainerImage{ID: imageID},
			State:    workloadmeta.ContainerState{Running: true},
		}

		store.Set(imageEntity)
		store.Set(runtimeOnly)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: imageEntity},
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: runtimeOnly},
		))

		// The kubelet describes the container too and wins the merge, so
		// Image.ID becomes the repo digest.
		store.Set(runningContainer)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: runningContainer},
		))

		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)
		counter.Store(0)

		// The container goes away and a fresh SBOM arrives for the image.
		store.Unset(runningContainer)
		store.Set(rescannedImageEntity)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: rescannedImageEntity},
			workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: runningContainer},
		))
		p.stop()

		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)

		assertSBOMSent(t, sender, makeExpectedSBOM(false, 2))
	})

	// Test 5: workloadmeta drops a whole event bundle when a subscriber is too
	// slow to read it, so the container removal is never delivered. The state
	// of the store is still the truth.
	t.Run("missed container event does not leave the image in use", func(t *testing.T) {
		p, sender, counter, store := newTestSetup(t)

		store.Set(imageEntity)
		store.Set(runningContainer)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: imageEntity},
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: runningContainer},
		))
		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)
		counter.Store(0)

		// The container is removed but the event never reaches the check.
		store.Unset(runningContainer)
		store.Set(rescannedImageEntity)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: rescannedImageEntity},
		))
		p.stop()

		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)

		assertSBOMSent(t, sender, makeExpectedSBOM(false, 2))
	})

	// Test 6: a periodic refresh emits SBOMs without an event bundle behind it,
	// so it is the only thing that tells the back end an image whose container
	// stop was never delivered is no longer in use. The set of images last
	// reported in use has to follow, or the container that starts the image
	// again looks like one that changes nothing and is never reported.
	t.Run("container starting an image a refresh reported idle is reported", func(t *testing.T) {
		p, sender, counter, store := newTestSetup(t)

		store.Set(imageEntity)
		store.Set(runningContainer)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: imageEntity},
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: runningContainer},
		))
		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)
		counter.Store(0)

		// The container goes away and the bundle saying so is dropped, so the
		// refresh is what reports inUse=false. It is rescanned meanwhile, which
		// tells the payloads emitted before and after this point apart.
		store.Unset(runningContainer)
		store.Set(rescannedImageEntity)

		refresher := newBatchRefresher(time.Hour, store, p)
		defer refresher.stop()
		refresher.step()

		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)
		assertSBOMSent(t, sender, makeExpectedSBOM(false, 2))
		counter.Store(0)

		// A container starts the image again.
		store.Set(runningContainer)
		p.processContainerImagesEvents(makeBundle(
			workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: runningContainer},
		))
		p.stop()

		assert.Eventually(t, func() bool { return counter.Load() == 1 }, time.Second, 5*time.Millisecond)

		assertSBOMSent(t, sender, makeExpectedSBOM(true, 2))
	})
}

// TestCorruptedSBOM checks that an image whose stored SBOM cannot be
// uncompressed is skipped. Uncompressing returns no SBOM alongside its error,
// so reading the SBOM anyway panicked, and the check runs on a goroutine that
// nothing recovers.
func TestCorruptedSBOM(t *testing.T) {
	imageEntity := &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
		},
		RepoTags:    []string{"datadog/agent:7-rc"},
		RepoDigests: []string{"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409"},
		SBOM: &workloadmeta.CompressedSBOM{
			Bom:    []byte("not a gzip stream"),
			Status: workloadmeta.Success,
		},
	}

	cacheDir := t.TempDir()
	cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{
		"sbom.cache_directory":         cacheDir,
		"sbom.container_image.enabled": true,
	})
	if sbomscanner.GetGlobalScanner() == nil {
		wmeta := fxutil.Test[option.Option[workloadmeta.Component]](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))
		_, err := sbomscanner.CreateGlobalScanner(cfg, wmeta)
		assert.Nil(t, err)
	}

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() configcomp.Component { return configcomp.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	sent := atomic.NewInt32(0)
	sender := mocksender.NewMockSender(t, "")
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Run(func(_ mock.Arguments) {
		sent.Inc()
	})

	p, err := newProcessor(store, workloadfilterfxmock.SetupMockFilter(t), sender, taggerfxmock.SetupFakeTagger(t), cfg, 1, 50*time.Millisecond, time.Second)
	assert.Nil(t, err)

	store.Set(imageEntity)
	p.processContainerImagesEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: imageEntity}},
		Ch:     make(chan struct{}),
	})
	p.stop()

	assert.Never(t, func() bool { return sent.Load() > 0 }, 100*time.Millisecond, 5*time.Millisecond)
}

func mustCompressSBOM(t *testing.T, sbom *workloadmeta.SBOM) *workloadmeta.CompressedSBOM {
	t.Helper()

	csbom, err := sbomutil.CompressSBOM(sbom)
	assert.Nil(t, err)

	return csbom
}
