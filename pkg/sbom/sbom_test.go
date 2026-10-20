// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
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
			name: "observation start alone",
			bom: &cyclonedx_v1_4.Bom{
				Metadata: &cyclonedx_v1_4.Metadata{Properties: []*cyclonedx_v1_4.Property{
					property(UsageObservedSinceProperty, "1700000000"),
				}},
				Components: []*cyclonedx_v1_4.Component{component("bash")},
			},
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

// TestSetHostUsageKeepsLatest checks that a host report the sbom check has yet
// to take gives way to the next one, so the check reads the latest report and
// the remote SBOM collector goes on at once.
func TestSetHostUsageKeepsLatest(t *testing.T) {
	older := &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{component("bash")}}
	newer := &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{component("openssl")}}

	SetHostUsage(older)
	SetHostUsage(newer)

	select {
	case got := <-HostUsage():
		assert.Same(t, newer, got)
	default:
		t.Fatal("no host report to take")
	}

	select {
	case got := <-HostUsage():
		t.Errorf("host report %v left behind", got)
	default:
	}
}

func TestUsageEnrichmentEnabled(t *testing.T) {
	for _, usage := range []bool{false, true} {
		agentConfig := configmock.New(t)
		agentConfig.SetInTest("sbom.enrichment.usage.enabled", usage)

		// system-probe reports usage on Linux alone.
		want := usage && runtime.GOOS == "linux"
		assert.Equal(t, want, UsageEnrichmentEnabled(agentConfig), "sbom.enrichment.usage.enabled: %t", usage)
	}
}
