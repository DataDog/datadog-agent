// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sbom

import (
	"strconv"
	"testing"
	"time"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	model "github.com/DataDog/agent-payload/v5/sbom"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/mock"
)

func TestProcessEvents(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID(""))
	sender.On("SBOM", mock.Anything, mock.Anything).Return()
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
						CycloneDXBOM: &cyclonedx.BOM{
							SpecVersion: "1.4",
							Version:     42,
							Components: &[]cyclonedx.Component{
								{
									Name: strconv.Itoa(100 * i),
								},
								{
									Name: strconv.Itoa(100*i + 1),
								},
								{
									Name: strconv.Itoa(100*i + 2),
								},
							},
						},
					},
				},
			},
			Ch: make(chan struct{}),
		})
	}

	sender.AssertNumberOfCalls(t, "SBOM", 1)
	sender.AssertSBOM(t, []model.SBOMPayload{
		{
			Version: 1,
			Source:  &sourceAgent,
			Entities: []*model.SBOMEntity{
				{
					Type:  model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:    "0",
					InUse: true,
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
							Components: []*cyclonedx_v1_4.Component{
								{
									Name: "0",
								},
								{
									Name: "1",
								},
								{
									Name: "2",
								},
							},
						},
					},
				},
				{
					Type:  model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:    "1",
					InUse: true,
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
							Components: []*cyclonedx_v1_4.Component{
								{
									Name: "100",
								},
								{
									Name: "101",
								},
								{
									Name: "102",
								},
							},
						},
					},
				},
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	sender.AssertNumberOfCalls(t, "SBOM", 2)
	sender.AssertSBOM(t, []model.SBOMPayload{
		{
			Version: 1,
			Source:  &sourceAgent,
			Entities: []*model.SBOMEntity{
				{
					Type:  model.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
					Id:    "2",
					InUse: true,
					Sbom: &model.SBOMEntity_Cyclonedx{
						Cyclonedx: &cyclonedx_v1_4.Bom{
							SpecVersion: "1.4",
							Version:     pointer.Ptr(int32(42)),
							Components: []*cyclonedx_v1_4.Component{
								{
									Name: "200",
								},
								{
									Name: "201",
								},
								{
									Name: "202",
								},
							},
						},
					},
				},
			},
		},
	})

	p.stop()
}
