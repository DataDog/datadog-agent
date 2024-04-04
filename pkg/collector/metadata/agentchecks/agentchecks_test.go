// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentchecks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
)

func TestExternalHostTags(t *testing.T) {
	host1 := "localhost"
	host2 := "127.0.0.1"
	sourceType := "vsphere"
	tags1 := []string{"foo", "bar"}
	tags2 := []string{"baz"}
	eTags1 := externalhost.ExternalTags{sourceType: tags1}
	eTags2 := externalhost.ExternalTags{sourceType: tags2}
	externalhost.SetExternalTags(host1, sourceType, tags1)
	externalhost.SetExternalTags(host2, sourceType, tags2)

	pl := GetPayload(context.Background())
	hpl := pl.ExternalHostPayload.Payload
	assert.Len(t, hpl, 2)
	for _, elem := range hpl {
		if elem[0] == host1 {
			assert.Equal(t, eTags1, elem[1])
		} else if elem[0] == host2 {
			assert.Equal(t, eTags2, elem[1])
		} else {
			assert.Fail(t, "Unexpected value for hostname: %s", elem[0])
		}
	}
}
