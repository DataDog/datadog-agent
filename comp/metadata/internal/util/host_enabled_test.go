// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestHostMetadataEnabled(t *testing.T) {
	for _, agentFlavor := range []string{flavor.Dogstatsd, flavor.DefaultAgent} {
		for _, aas := range []bool{false, true} {
			t.Run(agentFlavor+"/"+map[bool]string{false: "host", true: "aas"}[aas], func(t *testing.T) {
				original := flavor.GetFlavor()
				flavor.SetFlavor(agentFlavor)
				t.Cleanup(func() { flavor.SetFlavor(original) })
				conf := configmock.New(t)
				conf.SetInTest("serverless.aas_extension_metadata_enabled", aas)
				assert.Equal(t, agentFlavor != flavor.Dogstatsd || !aas, HostMetadataEnabled(conf))
			})
		}
	}
}
