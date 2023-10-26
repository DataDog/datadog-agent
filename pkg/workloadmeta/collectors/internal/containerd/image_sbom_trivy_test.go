// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build containerd && trivy && test

package containerd

import (
	"reflect"
	"testing"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	trivydx "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
)

func Test_updateSBOMMetadata(t *testing.T) {
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
					CycloneDXBOM: &cyclonedx.BOM{
						Metadata: &cyclonedx.Metadata{
							Component: &cyclonedx.Component{Properties: nil},
						},
					},
				},
				repoTags:    []string{"tag1"},
				repoDigests: []string{"digest1"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Success,
				CycloneDXBOM: &cyclonedx.BOM{
					Metadata: &cyclonedx.Metadata{
						Component: &cyclonedx.Component{Properties: nil},
					},
				},
			},
		},
		{
			name: "missing repoTags and repoDigests",
			args: args{
				sbom: &workloadmeta.SBOM{
					Status: workloadmeta.Success,
					CycloneDXBOM: &cyclonedx.BOM{
						Metadata: &cyclonedx.Metadata{
							Component: &cyclonedx.Component{
								Properties: &[]cyclonedx.Property{
									{Name: trivydx.Namespace + trivydx.PropertyRepoTag, Value: "tag2"},
									{Name: trivydx.Namespace + trivydx.PropertyRepoDigest, Value: "digest2"},
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
				CycloneDXBOM: &cyclonedx.BOM{
					Metadata: &cyclonedx.Metadata{
						Component: &cyclonedx.Component{
							Properties: &[]cyclonedx.Property{
								{Name: trivydx.Namespace + trivydx.PropertyRepoTag, Value: "tag2"},
								{Name: trivydx.Namespace + trivydx.PropertyRepoDigest, Value: "digest2"},
								{Name: trivydx.Namespace + trivydx.PropertyRepoTag, Value: "tag1"},
								{Name: trivydx.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
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
					CycloneDXBOM: &cyclonedx.BOM{
						Metadata: &cyclonedx.Metadata{
							Component: &cyclonedx.Component{
								Properties: &[]cyclonedx.Property{
									{Name: trivydx.Namespace + trivydx.PropertyRepoTag, Value: "tag1"},
									{Name: trivydx.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
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
				CycloneDXBOM: &cyclonedx.BOM{
					Metadata: &cyclonedx.Metadata{
						Component: &cyclonedx.Component{
							Properties: &[]cyclonedx.Property{
								{Name: trivydx.Namespace + trivydx.PropertyRepoTag, Value: "tag1"},
								{Name: trivydx.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updateSBOMMetadata(tt.args.sbom, tt.args.repoTags, tt.args.repoDigests); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("updateSBOMMetadata() = %v, want %v", got, tt.want)
			}
		})
	}
}
