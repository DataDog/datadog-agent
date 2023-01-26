// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

import (
	"fmt"
	"strconv"
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

	for i := 0; i < 3; i++ {
		p.processEvents(workloadmeta.EventBundle{
			Events: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.ContainerImageMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainerImageMetadata,
							ID:   strconv.Itoa(i),
						},
						ShortName:    fmt.Sprintf("short_name_%d", i),
						RepoTags:     []string{"tag_1", "tag_2"},
						RepoDigests:  []string{fmt.Sprintf("digest_%d", i)},
						SizeBytes:    42,
						OS:           "DOS",
						OSVersion:    "6.22",
						Architecture: "80486DX",
						Layers: []workloadmeta.ContainerImageLayer{
							{
								MediaType: "media",
								Digest:    fmt.Sprintf("digest_layer_1_%d", i),
								SizeBytes: 43,
								URLs:      []string{"url"},
								History: v1.History{
									Created: pointer.Ptr(time.Unix(42, 43)),
								},
							},
							{
								MediaType: "media",
								Digest:    fmt.Sprintf("digest_layer_2_%d", i),
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
	}

	sender.AssertNumberOfCalls(t, "ContainerImage", 1)
	sender.AssertContainerImage(t, []model.ContainerImagePayload{
		{
			Version: "v1",
			Images: []*model.ContainerImage{
				{
					Id:          "0",
					ShortName:   "short_name_0",
					Tags:        []string{"tag_1", "tag_2"},
					Digest:      "0",
					Size:        42,
					RepoDigests: []string{"digest_0"},
					Os: &model.ContainerImage_OperatingSystem{
						Name:         "DOS",
						Version:      "6.22",
						Architecture: "80486DX",
					},
					Layers: []*model.ContainerImage_ContainerImageLayer{
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_1_0",
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
							Digest:    "digest_layer_2_0",
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
					Id:          "1",
					ShortName:   "short_name_1",
					Tags:        []string{"tag_1", "tag_2"},
					Digest:      "1",
					Size:        42,
					RepoDigests: []string{"digest_1"},
					Os: &model.ContainerImage_OperatingSystem{
						Name:         "DOS",
						Version:      "6.22",
						Architecture: "80486DX",
					},
					Layers: []*model.ContainerImage_ContainerImageLayer{
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_1_1",
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
							Digest:    "digest_layer_2_1",
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
					Id:          "2",
					ShortName:   "short_name_2",
					Tags:        []string{"tag_1", "tag_2"},
					Digest:      "2",
					Size:        42,
					RepoDigests: []string{"digest_2"},
					Os: &model.ContainerImage_OperatingSystem{
						Name:         "DOS",
						Version:      "6.22",
						Architecture: "80486DX",
					},
					Layers: []*model.ContainerImage_ContainerImageLayer{
						{
							Urls:      []string{"url"},
							MediaType: "media",
							Digest:    "digest_layer_1_2",
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
							Digest:    "digest_layer_2_2",
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
