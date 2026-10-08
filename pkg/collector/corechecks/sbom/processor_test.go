// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || windows

package sbom

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	model "github.com/DataDog/agent-payload/v5/sbom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	"github.com/DataDog/datadog-agent/pkg/sbom"
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

// hostReport is the report of a host scan: the BOM of the host and the hash of
// the packages it lists.
type hostReport struct {
	id  string
	bom *cyclonedx_v1_4.Bom
}

func (r hostReport) ToCycloneDX() *cyclonedx_v1_4.Bom { return r.bom }

func (r hostReport) ID() string { return r.id }

// newHostScan returns a successful scan, taken at createdAt, of a host holding
// bash.
func newHostScan(createdAt time.Time) sbom.ScanResult {
	return sbom.ScanResult{
		Report: hostReport{
			id: "sha256:packages",
			bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{{
				Name:    "bash",
				Version: "5.2.26-6.el10",
				Purl:    pointer.Ptr("pkg:rpm/redhat/bash@5.2.26-6.el10"),
			}}},
		},
		CreatedAt: createdAt,
		Duration:  time.Second,
	}
}

// lastSeenRunning returns the LastSeenRunning property of the named component
// of bom, or the empty string.
func lastSeenRunning(bom *cyclonedx_v1_4.Bom, name string) string {
	for _, comp := range bom.GetComponents() {
		if comp.GetName() != name {
			continue
		}
		for _, prop := range comp.GetProperties() {
			if prop.GetName() == sbom.LastAccessProperty {
				return prop.GetValue()
			}
		}
	}
	return ""
}

// TestProcessHostUsage checks that a runtime usage report of the host rides the
// next host scan, which goes out in full, and that heartbeats resume once the
// usage holds still. A heartbeat tells the back end that the SBOM it holds is
// current, and a new report makes that SBOM stale.
func TestProcessHostUsage(t *testing.T) {
	usage := &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{{
		Name:    "bash",
		Version: "5.2.26-6.el10",
		Properties: []*cyclonedx_v1_4.Property{
			{Name: sbom.LastAccessProperty, Value: pointer.Ptr("1700000000")},
			{Name: sbom.HasSetSuidBitProperty, Value: pointer.Ptr("false")},
			{Name: sbom.RunningAsRootProperty, Value: pointer.Ptr("true")},
		},
	}}}
	scanned := time.Unix(1700000000, 0)

	newHostProcessor := func() *processor {
		return &processor{
			queue:                 make(chan *model.SBOMEntity, 4),
			hostname:              "host",
			hostHeartbeatValidity: time.Hour,
		}
	}

	t.Run("after a scan", func(t *testing.T) {
		p := newHostProcessor()

		p.processHostScanResult(newHostScan(scanned))
		entity := <-p.queue
		require.NotNil(t, entity.GetCyclonedx(), "the first scan goes out in full")
		assert.Empty(t, lastSeenRunning(entity.GetCyclonedx(), "bash"))

		p.processHostUsage(usage)
		assert.Empty(t, p.queue, "a report waits for the next scan")

		p.processHostScanResult(newHostScan(scanned.Add(time.Minute)))
		entity = <-p.queue
		assert.False(t, entity.GetHeartbeat())
		require.NotNil(t, entity.GetCyclonedx(), "the scan after a report goes out in full")
		assert.Equal(t, "1700000000", lastSeenRunning(entity.GetCyclonedx(), "bash"))
		assert.Equal(t, "sha256:packages", entity.GetHash())

		p.processHostScanResult(newHostScan(scanned.Add(2 * time.Minute)))
		entity = <-p.queue
		assert.True(t, entity.GetHeartbeat(), "an unchanged scan after that is a heartbeat")
		assert.Nil(t, entity.GetCyclonedx())
	})

	t.Run("before the first scan", func(t *testing.T) {
		p := newHostProcessor()

		p.processHostUsage(usage)
		assert.Empty(t, p.queue, "a report waits for the first scan")

		p.processHostScanResult(newHostScan(scanned))
		entity := <-p.queue
		require.NotNil(t, entity.GetCyclonedx())
		assert.Equal(t, "1700000000", lastSeenRunning(entity.GetCyclonedx(), "bash"))
	})
}

// fakeScanner records the scan requests it receives.
type fakeScanner struct {
	requests []sbom.ScanRequest
}

func (s *fakeScanner) Scan(request sbom.ScanRequest) error {
	s.requests = append(s.requests, request)
	return nil
}

// TestHostSBOMWaitsForUsage checks that the first host SBOM waits for the
// runtime usage of the host, so that it goes out with it, and that it goes out
// as it is once the wait ends.
func TestHostSBOMWaitsForUsage(t *testing.T) {
	usage := &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{{
		Name:    "bash",
		Version: "5.2.26-6.el10",
		Properties: []*cyclonedx_v1_4.Property{
			{Name: sbom.LastAccessProperty, Value: pointer.Ptr("1700000000")},
			{Name: sbom.HasSetSuidBitProperty, Value: pointer.Ptr("false")},
			{Name: sbom.RunningAsRootProperty, Value: pointer.Ptr("true")},
		},
	}}}
	scanned := time.Unix(1700000000, 0)

	newHostProcessor := func() (*processor, *fakeScanner) {
		scanner := &fakeScanner{}
		return &processor{
			queue:                 make(chan *model.SBOMEntity, 4),
			sbomScanner:           scanner,
			hostSBOM:              true,
			hostname:              "host",
			hostHeartbeatValidity: time.Hour,
			usageEnrichment:       true,
		}, scanner
	}

	t.Run("a report sends the scan with its usage", func(t *testing.T) {
		p, scanner := newHostProcessor()

		p.processHostScanResult(newHostScan(scanned))
		p.releaseExpiredHolds(time.Now())
		assert.Empty(t, p.queue, "the first scan waits for the usage of the host")

		p.processHostUsage(usage)
		entity := <-p.queue
		require.NotNil(t, entity.GetCyclonedx())
		assert.Equal(t, "1700000000", lastSeenRunning(entity.GetCyclonedx(), "bash"))
		assert.Equal(t, "sha256:packages", entity.GetHash())

		p.processHostScanResult(newHostScan(scanned.Add(time.Minute)))
		entity = <-p.queue
		assert.True(t, entity.GetHeartbeat(), "an unchanged scan after that is a heartbeat")
		assert.Empty(t, scanner.requests)
	})

	t.Run("the wait ends without usage", func(t *testing.T) {
		p, scanner := newHostProcessor()

		p.processHostScanResult(newHostScan(scanned))
		p.releaseExpiredHolds(time.Now().Add(usageGracePeriod))
		entity := <-p.queue
		require.NotNil(t, entity.GetCyclonedx(), "the scan goes out once the wait ends")
		assert.Empty(t, lastSeenRunning(entity.GetCyclonedx(), "bash"))

		p.processHostUsage(usage)
		assert.Empty(t, p.queue)
		assert.Len(t, scanner.requests, 1, "the first report triggers a scan to carry it")

		p.processHostUsage(usage)
		assert.Len(t, scanner.requests, 1, "a later report waits for the next scan")
	})

	t.Run("the newest scan waits in place of the older one", func(t *testing.T) {
		p, _ := newHostProcessor()

		p.processHostScanResult(newHostScan(scanned))
		p.processHostScanResult(newHostScan(scanned.Add(time.Minute)))
		assert.Empty(t, p.queue)

		p.processHostUsage(usage)
		entity := <-p.queue
		assert.Equal(t, scanned.Add(time.Minute).Unix(), entity.GetGeneratedAt().AsTime().Unix())
		assert.Empty(t, p.queue)
	})

	t.Run("a failed scan goes out at once", func(t *testing.T) {
		p, _ := newHostProcessor()

		p.processHostScanResult(sbom.ScanResult{Error: errors.New("scan failed"), CreatedAt: scanned})
		entity := <-p.queue
		assert.Equal(t, model.SBOMStatus_FAILED, entity.GetStatus())

		p.processHostScanResult(newHostScan(scanned.Add(time.Minute)))
		assert.Empty(t, p.queue, "the first successful scan still waits")
	})
}

const graceImageID = "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd"

// graceImage returns the image graceImageID with an SBOM of the given status
// listing components.
func graceImage(t *testing.T, status workloadmeta.SBOMStatus, components ...*cyclonedx_v1_4.Component) *workloadmeta.ContainerImageMetadata {
	return &workloadmeta.ContainerImageMetadata{
		EntityID:    workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: graceImageID},
		RepoTags:    []string{"datadog/agent:7"},
		RepoDigests: []string{"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409"},
		SBOM: mustCompressSBOM(t, &workloadmeta.SBOM{
			CycloneDXBOM:   &cyclonedx_v1_4.Bom{Components: components},
			GenerationTime: time.Unix(1700000000, 0),
			Status:         status,
		}),
	}
}

// debPackage returns the deb package bash, with the given runtime properties.
func debPackage(properties ...*cyclonedx_v1_4.Property) *cyclonedx_v1_4.Component {
	return &cyclonedx_v1_4.Component{
		Name:       "bash",
		Version:    "5.2.15-2",
		Purl:       pointer.Ptr("pkg:deb/debian/bash@5.2.15-2?arch=amd64"),
		Properties: properties,
	}
}

// newImageGraceProcessor returns a processor whose first image SBOMs wait for
// their runtime usage, the store it reads, and the entities it sends.
func newImageGraceProcessor(t *testing.T) (*processor, workloadmetamock.Mock, <-chan *model.SBOMEntity) {
	cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{
		"sbom.cache_directory":         t.TempDir(),
		"sbom.container_image.enabled": true,
	})
	if sbomscanner.GetGlobalScanner() == nil {
		wmeta := fxutil.Test[option.Option[workloadmeta.Component]](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))
		_, err := sbomscanner.CreateGlobalScanner(cfg, wmeta)
		require.NoError(t, err)
	}

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() configcomp.Component { return configcomp.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	sent := make(chan *model.SBOMEntity, 16)
	sender := mocksender.NewMockSender(t, "")
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
		var payload model.SBOMPayload
		if assert.NoError(t, proto.Unmarshal(args.Get(0).([]byte), &payload)) {
			for _, entity := range payload.Entities {
				sent <- entity
			}
		}
	})

	p, err := newProcessor(store, workloadfilterfxmock.SetupMockFilter(t), sender, taggerfxmock.SetupFakeTagger(t), cfg, 1, 50*time.Millisecond, time.Second)
	require.NoError(t, err)
	p.usageEnrichment = true
	t.Cleanup(p.stop)

	return p, store, sent
}

// runGraceImage starts a container of the image graceImageID.
func runGraceImage(store workloadmetamock.Mock) *workloadmeta.Container {
	container := &workloadmeta.Container{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container"},
		EntityMeta: workloadmeta.EntityMeta{Name: "container"},
		Image:      workloadmeta.ContainerImage{ID: graceImageID},
		State:      workloadmeta.ContainerState{Running: true},
	}
	store.Set(container)
	return container
}

// notify hands p the events of a bundle, once the store reflects them.
func notify(p *processor, store workloadmetamock.Mock, events ...workloadmeta.Event) {
	for _, event := range events {
		switch event.Type {
		case workloadmeta.EventTypeSet:
			store.Set(event.Entity)
		case workloadmeta.EventTypeUnset:
			store.Unset(event.Entity)
		}
	}
	p.processContainerImagesEvents(workloadmeta.EventBundle{Events: events, Ch: make(chan struct{})})
}

func receiveSBOM(t *testing.T, sent <-chan *model.SBOMEntity) *model.SBOMEntity {
	t.Helper()
	select {
	case entity := <-sent:
		return entity
	case <-time.After(time.Second):
		t.Fatal("no SBOM went out")
		return nil
	}
}

func assertNoSBOM(t *testing.T, sent <-chan *model.SBOMEntity) {
	t.Helper()
	select {
	case entity := <-sent:
		t.Fatalf("SBOM %s went out", entity.GetId())
	case <-time.After(200 * time.Millisecond):
	}
}

// TestImageSBOMWaitsForUsage checks that the first SBOM of an image in use waits
// for the runtime usage of the image, so that it goes out with it, and that it
// goes out as it is once the wait ends.
func TestImageSBOMWaitsForUsage(t *testing.T) {
	used := debPackage(
		&cyclonedx_v1_4.Property{Name: sbom.LastAccessProperty, Value: pointer.Ptr("1700000000")},
		&cyclonedx_v1_4.Property{Name: sbom.HasSetSuidBitProperty, Value: pointer.Ptr("false")},
		&cyclonedx_v1_4.Property{Name: sbom.RunningAsRootProperty, Value: pointer.Ptr("true")},
	)
	set := func(entity workloadmeta.Entity) workloadmeta.Event {
		return workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: entity}
	}

	t.Run("the merged usage sends the SBOM", func(t *testing.T) {
		p, store, sent := newImageGraceProcessor(t)
		runGraceImage(store)

		notify(p, store, set(graceImage(t, workloadmeta.Success, debPackage())))
		assertNoSBOM(t, sent)

		notify(p, store, set(graceImage(t, workloadmeta.Success, used)))
		entity := receiveSBOM(t, sent)
		assert.True(t, entity.GetInUse())
		assert.True(t, sbom.IsEnriched(entity.GetCyclonedx()), "the SBOM goes out with its usage")
		assertNoSBOM(t, sent)
	})

	t.Run("the wait ends without usage", func(t *testing.T) {
		p, store, sent := newImageGraceProcessor(t)
		runGraceImage(store)

		notify(p, store, set(graceImage(t, workloadmeta.Success, debPackage())))
		p.releaseExpiredHolds(time.Now())
		assertNoSBOM(t, sent)

		p.releaseExpiredHolds(time.Now().Add(usageGracePeriod))
		entity := receiveSBOM(t, sent)
		assert.True(t, entity.GetInUse())
		assert.False(t, sbom.IsEnriched(entity.GetCyclonedx()))

		notify(p, store, set(graceImage(t, workloadmeta.Success, debPackage())))
		receiveSBOM(t, sent)
	})

	t.Run("a refresh leaves the waiting SBOM alone", func(t *testing.T) {
		p, store, sent := newImageGraceProcessor(t)
		runGraceImage(store)

		img := graceImage(t, workloadmeta.Success, debPackage())
		notify(p, store, set(img))
		p.processImageSBOM(img, runningImages(store))
		assertNoSBOM(t, sent)
	})

	t.Run("an unset image stops waiting", func(t *testing.T) {
		p, store, sent := newImageGraceProcessor(t)
		runGraceImage(store)

		img := graceImage(t, workloadmeta.Success, debPackage())
		notify(p, store, set(img))
		notify(p, store, workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: img})
		p.releaseExpiredHolds(time.Now().Add(usageGracePeriod))
		assertNoSBOM(t, sent)
	})

	t.Run("an image sent unused waits once a container runs it", func(t *testing.T) {
		p, store, sent := newImageGraceProcessor(t)

		notify(p, store, set(graceImage(t, workloadmeta.Success, debPackage())))
		assert.False(t, receiveSBOM(t, sent).GetInUse())

		notify(p, store, set(runGraceImage(store)))
		assertNoSBOM(t, sent)

		notify(p, store, set(graceImage(t, workloadmeta.Success, used)))
		entity := receiveSBOM(t, sent)
		assert.True(t, entity.GetInUse())
		assert.True(t, sbom.IsEnriched(entity.GetCyclonedx()), "the first SBOM in use goes out with its usage")
	})

	t.Run("an image left unused during the wait waits again", func(t *testing.T) {
		p, store, sent := newImageGraceProcessor(t)
		container := runGraceImage(store)

		notify(p, store, set(graceImage(t, workloadmeta.Success, debPackage())))
		assertNoSBOM(t, sent)

		store.Unset(container)
		p.releaseExpiredHolds(time.Now().Add(usageGracePeriod))
		assert.False(t, receiveSBOM(t, sent).GetInUse(), "the wait ends with the image unused")

		notify(p, store, set(runGraceImage(store)))
		assertNoSBOM(t, sent)

		notify(p, store, set(graceImage(t, workloadmeta.Success, used)))
		entity := receiveSBOM(t, sent)
		assert.True(t, entity.GetInUse())
		assert.True(t, sbom.IsEnriched(entity.GetCyclonedx()), "the SBOM in use goes out with its usage")
	})

	t.Run("an image sent in use goes out at once", func(t *testing.T) {
		p, store, sent := newImageGraceProcessor(t)
		runGraceImage(store)

		notify(p, store, set(graceImage(t, workloadmeta.Success, used)))
		receiveSBOM(t, sent)

		notify(p, store, set(graceImage(t, workloadmeta.Success, debPackage())))
		assert.False(t, sbom.IsEnriched(receiveSBOM(t, sent).GetCyclonedx()), "a rescan after the first SBOM in use goes out at once")
	})

	for _, tt := range []struct {
		name  string
		inUse bool
		img   *workloadmeta.ContainerImageMetadata
	}{
		{name: "not in use", img: graceImage(t, workloadmeta.Success, debPackage())},
		{name: "pending", inUse: true, img: graceImage(t, workloadmeta.Pending)},
		{name: "failed", inUse: true, img: graceImage(t, workloadmeta.Failed)},
		{name: "no OS package", inUse: true, img: graceImage(t, workloadmeta.Success, &cyclonedx_v1_4.Component{
			Name:    "lodash",
			Version: "4.17.21",
			Purl:    pointer.Ptr("pkg:npm/lodash@4.17.21"),
		})},
		{name: "already enriched", inUse: true, img: graceImage(t, workloadmeta.Success, used)},
	} {
		t.Run(tt.name+" goes out at once", func(t *testing.T) {
			p, store, sent := newImageGraceProcessor(t)
			if tt.inUse {
				runGraceImage(store)
			}

			notify(p, store, set(tt.img))
			receiveSBOM(t, sent)
		})
	}
}

// TestUsagePropertyNames checks that the merge and the producers of the runtime
// usage agree on its property names.
func TestUsagePropertyNames(t *testing.T) {
	assert.Equal(t, sbom.LastAccessProperty, sbomutil.LastAccessProperty)
	assert.Equal(t, sbom.HasSetSuidBitProperty, sbomutil.HasSetSuidBitProperty)
	assert.Equal(t, sbom.RunningAsRootProperty, sbomutil.RunningAsRootProperty)
	assert.Equal(t, sbom.UsageObservedSinceProperty, sbomutil.UsageObservedSinceProperty)
}

// usageReport returns a runtime usage report, recorded since since, of bash
// seen running and zsh unseen.
func usageReport(since time.Time) *cyclonedx_v1_4.Bom {
	used := func(name, lastSeen string) *cyclonedx_v1_4.Component {
		return &cyclonedx_v1_4.Component{
			Name:    name,
			Version: "5.2",
			Properties: []*cyclonedx_v1_4.Property{
				{Name: sbom.LastAccessProperty, Value: pointer.Ptr(lastSeen)},
				{Name: sbom.HasSetSuidBitProperty, Value: pointer.Ptr("false")},
				{Name: sbom.RunningAsRootProperty, Value: pointer.Ptr("false")},
			},
		}
	}

	bom := &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{used("bash", "1700000000"), used("zsh", "0")}}
	if !since.IsZero() {
		bom.Metadata = &cyclonedx_v1_4.Metadata{Properties: []*cyclonedx_v1_4.Property{
			{Name: sbom.UsageObservedSinceProperty, Value: pointer.Ptr(strconv.FormatInt(since.Unix(), 10))},
		}}
	}
	return bom
}

// shellPackages returns the rpm packages bash and zsh.
func shellPackages() []*cyclonedx_v1_4.Component {
	return []*cyclonedx_v1_4.Component{
		{Name: "bash", Version: "5.2", Purl: pointer.Ptr("pkg:rpm/redhat/bash@5.2")},
		{Name: "zsh", Version: "5.2", Purl: pointer.Ptr("pkg:rpm/redhat/zsh@5.2")},
	}
}

// TestHostSBOMHidesUnobservedDuringWindow checks that while the usage of the
// host was recorded for less than its window, an unseen package loses its
// runtime properties, and that the first scan after the window closes goes
// out in full. A package the report lacks loses them past the window too.
func TestHostSBOMHidesUnobservedDuringWindow(t *testing.T) {
	scan := func(createdAt time.Time) sbom.ScanResult {
		return sbom.ScanResult{
			Report:    hostReport{id: "sha256:packages", bom: &cyclonedx_v1_4.Bom{Components: shellPackages()}},
			CreatedAt: createdAt,
			Duration:  time.Second,
		}
	}
	newHostProcessor := func(window time.Duration) *processor {
		return &processor{
			queue:                 make(chan *model.SBOMEntity, 4),
			hostname:              "host",
			hostHeartbeatValidity: time.Hour,
			hostUsageWindow:       window,
		}
	}
	scanned := time.Unix(1700000000, 0)

	t.Run("the window closes", func(t *testing.T) {
		now := scanned
		p := newHostProcessor(time.Hour)
		p.clock = func() time.Time { return now }
		p.processHostUsage(usageReport(scanned))

		p.processHostScanResult(scan(scanned))
		entity := <-p.queue
		require.NotNil(t, entity.GetCyclonedx())
		assert.Equal(t, "1700000000", lastSeenRunning(entity.GetCyclonedx(), "bash"))
		assert.Empty(t, lastSeenRunning(entity.GetCyclonedx(), "zsh"), "zsh, not seen running yet, is unknown")

		now = scanned.Add(time.Hour - time.Second)
		p.processHostScanResult(scan(scanned.Add(time.Minute)))
		assert.True(t, (<-p.queue).GetHeartbeat(), "an unchanged scan in the window is a heartbeat")

		now = scanned.Add(time.Hour)
		p.processHostScanResult(scan(scanned.Add(2 * time.Minute)))
		entity = <-p.queue
		require.NotNil(t, entity.GetCyclonedx(), "the first scan after the window closes goes out in full")
		assert.Equal(t, "0", lastSeenRunning(entity.GetCyclonedx(), "zsh"))
	})

	t.Run("a package the report lacks reads unknown", func(t *testing.T) {
		p := newHostProcessor(time.Hour)
		p.clock = func() time.Time { return scanned.Add(2 * time.Hour) }
		p.processHostUsage(usageReport(scanned))

		packages := append(shellPackages(), &cyclonedx_v1_4.Component{Name: "fish", Version: "3.7", Purl: pointer.Ptr("pkg:rpm/redhat/fish@3.7")})
		p.processHostScanResult(sbom.ScanResult{
			Report:    hostReport{id: "sha256:packages", bom: &cyclonedx_v1_4.Bom{Components: packages}},
			CreatedAt: scanned.Add(2 * time.Hour),
			Duration:  time.Second,
		})
		entity := <-p.queue
		require.NotNil(t, entity.GetCyclonedx())
		assert.Equal(t, "0", lastSeenRunning(entity.GetCyclonedx(), "zsh"), "zsh, reported unseen, is unused past the window")
		assert.Empty(t, lastSeenRunning(entity.GetCyclonedx(), "fish"), "fish, which the report lacks, is unknown")
	})

	t.Run("a report without a start hides nothing", func(t *testing.T) {
		p := newHostProcessor(time.Hour)
		p.processHostUsage(usageReport(time.Time{}))

		p.processHostScanResult(scan(scanned))
		entity := <-p.queue
		require.NotNil(t, entity.GetCyclonedx())
		assert.Equal(t, "0", lastSeenRunning(entity.GetCyclonedx(), "zsh"))
	})
}

// TestImageSBOMHidesUnobservedDuringWindow checks that while the usage of an
// image was recorded for less than its window, an unseen package loses its
// runtime properties in the SBOM that goes out, and keeps them in the stored
// one.
func TestImageSBOMHidesUnobservedDuringWindow(t *testing.T) {
	merged := func(since time.Time) *workloadmeta.ContainerImageMetadata {
		bom := sbomutil.MergeRuntimeProperties(&cyclonedx_v1_4.Bom{Components: shellPackages()}, usageReport(since))
		img := graceImage(t, workloadmeta.Success)
		img.SBOM = mustCompressSBOM(t, &workloadmeta.SBOM{CycloneDXBOM: bom, GenerationTime: time.Unix(1700000000, 0), Status: workloadmeta.Success})
		return img
	}

	tests := []struct {
		name    string
		since   time.Time
		wantZsh string
	}{
		{name: "window open", since: time.Now(), wantZsh: ""},
		{name: "window closed", since: time.Now().Add(-2 * time.Hour), wantZsh: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, store, sent := newImageGraceProcessor(t)
			p.imageUsageWindow = time.Hour

			img := merged(tt.since)
			notify(p, store, workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: img})
			entity := receiveSBOM(t, sent)
			assert.Equal(t, "1700000000", lastSeenRunning(entity.GetCyclonedx(), "bash"))
			assert.Equal(t, tt.wantZsh, lastSeenRunning(entity.GetCyclonedx(), "zsh"))

			stored, err := store.GetImage(img.ID)
			require.NoError(t, err)
			bom, err := sbomutil.UncompressSBOM(stored.SBOM)
			require.NoError(t, err)
			assert.Equal(t, "0", lastSeenRunning(bom.CycloneDXBOM, "zsh"), "the stored SBOM keeps what the report says")
		})
	}
}
