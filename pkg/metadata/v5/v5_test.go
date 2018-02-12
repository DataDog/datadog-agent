// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package v5

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	python "github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
)

// Setup the test module
func TestMain(m *testing.M) {
	state := py.Initialize()

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	python.Finalize()

	os.Exit(ret)
}

func TestGetPayload(t *testing.T) {
	pl := GetPayload("testhostname")
	assert.NotNil(t, pl)
}

func TestExternalHostTags(t *testing.T) {
	host1 := "localhost"
	host2 := "127.0.0.1"
	eTags1 := externalhost.ExternalTags{"vsphere": []string{"foo", "bar"}}
	eTags2 := externalhost.ExternalTags{"vsphere": []string{"baz"}}
	externalhost.AddExternalTags(host1, eTags1)
	externalhost.AddExternalTags(host2, eTags2)

	pl := GetPayload("")
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
