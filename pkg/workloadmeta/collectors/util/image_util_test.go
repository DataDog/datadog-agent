// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build trivy && test

// Package util contains utility functions for image metadata collection
package util

import (
	"reflect"
	"testing"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	trivydx "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	trivydxcore "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx/core"
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
									{Name: trivydxcore.Namespace + trivydx.PropertyRepoTag, Value: "tag2"},
									{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest2"},
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
								{Name: trivydxcore.Namespace + trivydx.PropertyRepoTag, Value: "tag1"},
								{Name: trivydxcore.Namespace + trivydx.PropertyRepoTag, Value: "tag2"},
								{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
								{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest2"},
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
									{Name: trivydxcore.Namespace + trivydx.PropertyRepoTag, Value: "tag1"},
									{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
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
								{Name: trivydxcore.Namespace + trivydx.PropertyRepoTag, Value: "tag1"},
								{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
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
					CycloneDXBOM: &cyclonedx.BOM{
						Metadata: &cyclonedx.Metadata{
							Component: &cyclonedx.Component{
								Properties: &[]cyclonedx.Property{
									{Name: trivydxcore.Namespace + trivydx.PropertyRepoTag, Value: "tag1"},
									{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
								},
							},
						},
					},
				},
				repoDigests: []string{"digest1"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Success,
				CycloneDXBOM: &cyclonedx.BOM{
					Metadata: &cyclonedx.Metadata{
						Component: &cyclonedx.Component{
							Properties: &[]cyclonedx.Property{
								{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
							},
						},
					},
				},
			},
		},
		{
			name: "other properties are not touched",
			args: args{
				sbom: &workloadmeta.SBOM{
					Status: workloadmeta.Success,
					CycloneDXBOM: &cyclonedx.BOM{
						Metadata: &cyclonedx.Metadata{
							Component: &cyclonedx.Component{
								Properties: &[]cyclonedx.Property{
									{Name: "prop1", Value: "tag1"},
									{Name: trivydxcore.Namespace + trivydx.PropertyRepoTag, Value: "tag1"},
									{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
								},
							},
						},
					},
				},
				repoDigests: []string{"digest1"},
			},
			want: &workloadmeta.SBOM{
				Status: workloadmeta.Success,
				CycloneDXBOM: &cyclonedx.BOM{
					Metadata: &cyclonedx.Metadata{
						Component: &cyclonedx.Component{
							Properties: &[]cyclonedx.Property{
								{Name: "prop1", Value: "tag1"},
								{Name: trivydxcore.Namespace + trivydx.PropertyRepoDigest, Value: "digest1"},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UpdateSBOMRepoMetadata(tt.args.sbom, tt.args.repoTags, tt.args.repoDigests); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("UpdateSBOMRepoMetadata) = %v, want %v", got, tt.want)
			}
		})
	}
}
