// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/contimage"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/golang/protobuf/ptypes/timestamp"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/stretchr/testify/mock"
)

func TestProcessEvents(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID(""))
	sender.On("ContainerImage", mock.Anything, mock.Anything).Return()
	p := newProcessor(sender, 2, 50*time.Millisecond)

	p.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainerImageMetadata,
						ID:   "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					},
					Registry:  "registry guessed by workloadmeta is ignored",
					ShortName: "short name guessed by workloadmeta is ignored",
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
					SizeBytes:    42,
					OS:           "DOS",
					OSVersion:    "6.22",
					Architecture: "80486DX",
					Layers: []workloadmeta.ContainerImageLayer{
						{
							MediaType: "media",
							Digest:    "digest_layer_1",
							SizeBytes: 43,
							URLs:      []string{"url"},
							History: v1.History{
								Created: pointer.Ptr(time.Unix(42, 43)),
							},
						},
						{
							MediaType: "media",
							Digest:    "digest_layer_2",
							URLs:      []string{"url"},
							SizeBytes: 44,
							History: v1.History{
								Created: pointer.Ptr(time.Unix(43, 44)),
							},
						},
					},
				},
			},
		},
		Ch: make(chan struct{}),
	})

	sender.AssertNumberOfCalls(t, "ContainerImage", 1)
	sender.AssertContainerImage(t, []model.ContainerImagePayload{
		{
			Version: "v1",
			Images: []*model.ContainerImage{
				{
					Id:        "datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					Name:      "datadog/agent",
					Registry:  "",
					ShortName: "agent",
					Tags: []string{
						"7-rc",
						"7.41.1-rc.1",
					},
					Digest:      "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					Size:        42,
					RepoDigests: []string{"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409"},
					Os: &model.ContainerImage_OperatingSystem{
						Name:         "DOS",
						Version:      "6.22",
						Architecture: "80486DX",
					},
					Layers: []*model.ContainerImage_ContainerImageLayer{
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_1",
							Size:      43,
							History: &model.ContainerImage_ContainerImageLayer_History{
								Created: &timestamp.Timestamp{
									Seconds: 42,
									Nanos:   43,
								},
							},
						},
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_2",
							Size:      44,
							History: &model.ContainerImage_ContainerImageLayer_History{
								Created: &timestamp.Timestamp{
									Seconds: 43,
									Nanos:   44,
								},
							},
						},
					},
					BuiltAt: &timestamp.Timestamp{
						Seconds: 43,
						Nanos:   44,
					},
				},
				{
					Id:        "gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					Name:      "gcr.io/datadoghq/agent",
					Registry:  "gcr.io",
					ShortName: "agent",
					Tags: []string{
						"7-rc",
						"7.41.1-rc.1",
					},
					Digest:      "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					Size:        42,
					RepoDigests: []string{"gcr.io/datadoghq/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409"},
					Os: &model.ContainerImage_OperatingSystem{
						Name:         "DOS",
						Version:      "6.22",
						Architecture: "80486DX",
					},
					Layers: []*model.ContainerImage_ContainerImageLayer{
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_1",
							Size:      43,
							History: &model.ContainerImage_ContainerImageLayer_History{
								Created: &timestamp.Timestamp{
									Seconds: 42,
									Nanos:   43,
								},
							},
						},
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_2",
							Size:      44,
							History: &model.ContainerImage_ContainerImageLayer_History{
								Created: &timestamp.Timestamp{
									Seconds: 43,
									Nanos:   44,
								},
							},
						},
					},
					BuiltAt: &timestamp.Timestamp{
						Seconds: 43,
						Nanos:   44,
					},
				},
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	sender.AssertNumberOfCalls(t, "ContainerImage", 2)
	sender.AssertContainerImage(t, []model.ContainerImagePayload{
		{
			Version: "v1",
			Images: []*model.ContainerImage{
				{
					Id:        "public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					Name:      "public.ecr.aws/datadog/agent",
					Registry:  "public.ecr.aws",
					ShortName: "agent",
					Tags: []string{
						"7-rc",
						"7.41.1-rc.1",
					},
					Digest:      "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					Size:        42,
					RepoDigests: []string{"public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409"},
					Os: &model.ContainerImage_OperatingSystem{
						Name:         "DOS",
						Version:      "6.22",
						Architecture: "80486DX",
					},
					Layers: []*model.ContainerImage_ContainerImageLayer{
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_1",
							Size:      43,
							History: &model.ContainerImage_ContainerImageLayer_History{
								Created: &timestamp.Timestamp{
									Seconds: 42,
									Nanos:   43,
								},
							},
						},
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_2",
							Size:      44,
							History: &model.ContainerImage_ContainerImageLayer_History{
								Created: &timestamp.Timestamp{
									Seconds: 43,
									Nanos:   44,
								},
							},
						},
					},
					BuiltAt: &timestamp.Timestamp{
						Seconds: 43,
						Nanos:   44,
					},
				},
			},
		},
	})

	p.stop()
}
