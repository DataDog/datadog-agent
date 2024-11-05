// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

import (
	"context"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/contimage"
	"github.com/golang/protobuf/ptypes/timestamp"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestProcessEvents(t *testing.T) {
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	tests := []struct {
		name           string
		inputEvents    []workloadmeta.Event
		expectedImages []*model.ContainerImage
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
								History: &v1.History{
									Created: pointer.Ptr(time.Unix(42, 43)),
								},
							},
							{
								MediaType: "media",
								Digest:    "digest_layer_2",
								URLs:      []string{"url"},
								SizeBytes: 44,
								History: &v1.History{
									Created: pointer.Ptr(time.Unix(43, 44)),
								},
							},
						},
					},
				},
			},
			expectedImages: []*model.ContainerImage{
				{
					Id: "datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:datadog/agent",
						"short_image:agent",
						"image_tag:7-rc",
						"image_tag:7.41.1-rc.1",
					},
					Name:      "datadog/agent",
					Registry:  "",
					ShortName: "agent",
					RepoTags: []string{
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
					Id: "gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:gcr.io/datadoghq/agent",
						"short_image:agent",
						"image_tag:7-rc",
						"image_tag:7.41.1-rc.1",
					},
					Name:      "gcr.io/datadoghq/agent",
					Registry:  "gcr.io",
					ShortName: "agent",
					RepoTags: []string{
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
				{
					Id: "public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:public.ecr.aws/datadog/agent",
						"short_image:agent",
						"image_tag:7-rc",
						"image_tag:7.41.1-rc.1",
					},
					Name:      "public.ecr.aws/datadog/agent",
					Registry:  "public.ecr.aws",
					ShortName: "agent",
					RepoTags: []string{
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
								History: &v1.History{
									Created: pointer.Ptr(time.Unix(42, 43)),
								},
							},
							{
								MediaType: "media",
								Digest:    "digest_layer_2",
								URLs:      []string{"url"},
								SizeBytes: 44,
								History: &v1.History{
									Created: pointer.Ptr(time.Unix(43, 44)),
								},
							},
						},
					},
				},
			},
			expectedImages: []*model.ContainerImage{
				{
					Id: "public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:public.ecr.aws/datadog/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:public.ecr.aws/datadog/agent",
						"short_image:agent",
						"image_tag:7-rc",
					},
					Name:      "public.ecr.aws/datadog/agent",
					Registry:  "public.ecr.aws",
					ShortName: "agent",
					RepoTags: []string{
						"7-rc",
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
				{
					Id: "gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					DdTags: []string{
						"image_id:gcr.io/datadoghq/agent@sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
						"image_name:gcr.io/datadoghq/agent",
						"short_image:agent",
						"image_tag:7-rc",
					},
					Name:      "gcr.io/datadoghq/agent",
					Registry:  "gcr.io",
					ShortName: "agent",
					RepoTags: []string{
						"7-rc",
					},
					Digest:      "sha256:9634b84c45c6ad220c3d0d2305aaa5523e47d6d43649c9bbeda46ff010b4aacd",
					Size:        42,
					RepoDigests: []string{},
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			imagesSent := atomic.NewInt32(0)

			sender := mocksender.NewMockSender("")
			sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Run(func(_ mock.Arguments) {
				imagesSent.Inc()
			})

			// Define a max size of 1 for the queue. With a size > 1, it's difficult to
			// control the number of events sent on each call.
			p := newProcessor(sender, 1, 50*time.Millisecond, fakeTagger)

			p.processEvents(workloadmeta.EventBundle{
				Events: test.inputEvents,
				Ch:     make(chan struct{}),
			})

			p.stop()

			// The queue is processing the events in a different go routine and might
			// need some time
			assert.Eventually(t, func() bool {
				return imagesSent.Load() == int32(len(test.expectedImages))
			}, 1*time.Second, 5*time.Millisecond)

			hname, _ := hostname.Get(context.TODO())
			for _, expectedImage := range test.expectedImages {
				encoded, err := proto.Marshal(&model.ContainerImagePayload{
					Version: "v1",
					Host:    hname,
					Source:  &sourceAgent,
					Images:  []*model.ContainerImage{expectedImage},
				})
				assert.Nil(t, err)
				sender.AssertEventPlatformEvent(t, encoded, eventplatform.EventTypeContainerImages)
			}
		})
	}
}
