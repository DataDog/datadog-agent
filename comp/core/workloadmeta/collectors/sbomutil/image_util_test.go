// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build trivy

package sbomutil

import (
	"reflect"
	"sort"
	"testing"

	trivycore "github.com/aquasecurity/trivy/pkg/sbom/core"
	trivydx "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func Test_UpdateSBOMRepoMetadata(t *testing.T) {
	type args struct {
		sbom        *workloadmeta.SBOM
		repoTags    []string
		repoDigests []string
	}
	tests := []struct {
		name string
		args args
		want *workloadmeta.SBOM
	}{
		{
			name: "status not success",
			args: args{
				sbom: &workloadmeta.SBOM{
					Status: workloadmeta.Failed,
				},
				repoTags:    []string{"tag1"},
				repoDigests: []string{"digest1"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Failed,
			},
		},
		{
			name: "properties is nil",
			args: args{
				sbom: &workloadmeta.SBOM{
					Status: workloadmeta.Success,
					CycloneDXBOM: &cyclonedx_v1_4.Bom{
						Metadata: &cyclonedx_v1_4.Metadata{
							Component: &cyclonedx_v1_4.Component{Properties: nil},
						},
					},
				},
				repoTags:    []string{"tag1"},
				repoDigests: []string{"digest1"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Success,
				CycloneDXBOM: &cyclonedx_v1_4.Bom{
					Metadata: &cyclonedx_v1_4.Metadata{
						Component: &cyclonedx_v1_4.Component{Properties: nil},
					},
				},
			},
		},
		{
			name: "missing repoTags and repoDigests",
			args: args{
				sbom: &workloadmeta.SBOM{
					Status: workloadmeta.Success,
					CycloneDXBOM: &cyclonedx_v1_4.Bom{
						Metadata: &cyclonedx_v1_4.Metadata{
							Component: &cyclonedx_v1_4.Component{
								Properties: []*cyclonedx_v1_4.Property{
									{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest2")},
									{Name: trivydx.Namespace + trivycore.PropertyRepoTag, Value: pointer.Ptr("tag2")},
								},
							},
						},
					},
				},
				repoTags:    []string{"tag1", "tag2"},
				repoDigests: []string{"digest1", "digest2"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Success,
				CycloneDXBOM: &cyclonedx_v1_4.Bom{
					Metadata: &cyclonedx_v1_4.Metadata{
						Component: &cyclonedx_v1_4.Component{
							Properties: []*cyclonedx_v1_4.Property{
								{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest1")},
								{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest2")},
								{Name: trivydx.Namespace + trivycore.PropertyRepoTag, Value: pointer.Ptr("tag1")},
								{Name: trivydx.Namespace + trivycore.PropertyRepoTag, Value: pointer.Ptr("tag2")},
							},
						},
					},
				},
			},
		},
		{
			name: "nothing is missing",
			args: args{
				sbom: &workloadmeta.SBOM{
					Status: workloadmeta.Success,
					CycloneDXBOM: &cyclonedx_v1_4.Bom{
						Metadata: &cyclonedx_v1_4.Metadata{
							Component: &cyclonedx_v1_4.Component{
								Properties: []*cyclonedx_v1_4.Property{
									{Name: trivydx.Namespace + trivycore.PropertyRepoTag, Value: pointer.Ptr("tag1")},
									{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest1")},
								},
							},
						},
					},
				},
				repoTags:    []string{"tag1"},
				repoDigests: []string{"digest1"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Success,
				CycloneDXBOM: &cyclonedx_v1_4.Bom{
					Metadata: &cyclonedx_v1_4.Metadata{
						Component: &cyclonedx_v1_4.Component{
							Properties: []*cyclonedx_v1_4.Property{
								{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest1")},
								{Name: trivydx.Namespace + trivycore.PropertyRepoTag, Value: pointer.Ptr("tag1")},
							},
						},
					},
				},
			},
		},
		{
			name: "a tag is removed",
			args: args{
				sbom: &workloadmeta.SBOM{
					Status: workloadmeta.Success,
					CycloneDXBOM: &cyclonedx_v1_4.Bom{
						Metadata: &cyclonedx_v1_4.Metadata{
							Component: &cyclonedx_v1_4.Component{
								Properties: []*cyclonedx_v1_4.Property{
									{Name: trivydx.Namespace + trivycore.PropertyRepoTag, Value: pointer.Ptr("tag1")},
									{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest1")},
								},
							},
						},
					},
				},
				repoDigests: []string{"digest1"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Success,
				CycloneDXBOM: &cyclonedx_v1_4.Bom{
					Metadata: &cyclonedx_v1_4.Metadata{
						Component: &cyclonedx_v1_4.Component{
							Properties: []*cyclonedx_v1_4.Property{
								{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest1")},
							},
						},
					},
				},
			},
		},
		{
			name: "other properties are still there",
			args: args{
				sbom: &workloadmeta.SBOM{
					Status: workloadmeta.Success,
					CycloneDXBOM: &cyclonedx_v1_4.Bom{
						Metadata: &cyclonedx_v1_4.Metadata{
							Component: &cyclonedx_v1_4.Component{
								Properties: []*cyclonedx_v1_4.Property{
									{Name: trivydx.Namespace + trivycore.PropertyRepoTag, Value: pointer.Ptr("tag1")},
									{Name: "prop1", Value: pointer.Ptr("tag1")},
									{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest1")},
								},
							},
						},
					},
				},
				repoDigests: []string{"digest1"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Success,
				CycloneDXBOM: &cyclonedx_v1_4.Bom{
					Metadata: &cyclonedx_v1_4.Metadata{
						Component: &cyclonedx_v1_4.Component{
							Properties: []*cyclonedx_v1_4.Property{
								{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest1")},
								{Name: "prop1", Value: pointer.Ptr("tag1")},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got := UpdateSBOMRepoMetadata(tt.args.sbom, tt.args.repoTags, tt.args.repoDigests)
			if got != nil &&
				got.CycloneDXBOM != nil &&
				got.CycloneDXBOM.Metadata != nil &&
				got.CycloneDXBOM.Metadata.Component != nil &&
				got.CycloneDXBOM.Metadata.Component.Properties != nil {
				// Sort properties to ensure consistent ordering for tests
				props := got.CycloneDXBOM.Metadata.Component.Properties
				sort.Slice(props, func(i, j int) bool {
					if props[i].Name == props[j].Name {
						return *props[i].Value < *props[j].Value
					}
					return props[i].Name < props[j].Name
				})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UpdateSBOMRepoMetadata) = %v, want %v", got, tt.want)
			}
		})
	}
}
