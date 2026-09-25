// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
)

func component(name string, properties ...*cyclonedx_v1_4.Property) *cyclonedx_v1_4.Component {
	return &cyclonedx_v1_4.Component{Name: name, Properties: properties}
}

func property(name, value string) *cyclonedx_v1_4.Property {
	return &cyclonedx_v1_4.Property{Name: name, Value: &value}
}

func TestIsEnriched(t *testing.T) {
	tests := []struct {
		name string
		bom  *cyclonedx_v1_4.Bom
		want bool
	}{
		{
			name: "nil bom",
			bom:  nil,
			want: false,
		},
		{
			name: "no component",
			bom:  &cyclonedx_v1_4.Bom{},
			want: false,
		},
		{
			name: "straight out of a scan",
			bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{
				component("openssl", property("PkgType", "debian")),
				component("bash"),
			}},
			want: false,
		},
		{
			name: "enriched",
			bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{
				component("openssl", property(LastAccessProperty, "1700000000")),
				component("bash", property(LastAccessProperty, "0")),
			}},
			want: true,
		},
		{
			name: "enriched, nil component first",
			bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{
				nil,
				component("bash", property(HasSetSuidBitProperty, "false"), property(LastAccessProperty, "0")),
			}},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, IsEnriched(test.bom))
		})
	}
}
