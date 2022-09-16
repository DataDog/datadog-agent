// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudfoundry

import (
	"testing"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/gardenfakes"
	"github.com/stretchr/testify/assert"
)

func TestListContainers(t *testing.T) {
	cli := gardenfakes.FakeClient{}
	bulkContainers := map[string]garden.ContainerInfoEntry{
		"ok": {
			Info: garden.ContainerInfo{
				State: "active",
			},
			Err: nil,
		},
		"ok err metrics": {
			Info: garden.ContainerInfo{
				State: "active",
			},
			Err: nil,
		},
		"not ok": {
			Info: garden.ContainerInfo{
				State: "on fire",
			},
			Err: garden.NewError("problem!"),
		},
	}
	metrics := map[string]garden.ContainerMetricsEntry{
		"ok":             {},
		"ok err metrics": {Err: garden.NewError("another problem!")},
		"not ok":         {},
	}
	okc := gardenfakes.FakeContainer{}
	okc.HandleReturns("ok")
	oknometricsc := gardenfakes.FakeContainer{}
	oknometricsc.HandleReturns("ok err metrics")
	nokc := gardenfakes.FakeContainer{}
	nokc.HandleReturns("not ok")
	containers := []garden.Container{
		&okc, &oknometricsc, &nokc,
	}

	cli.BulkInfoReturns(bulkContainers, nil)
	cli.BulkMetricsReturns(metrics, nil)
	cli.ContainersReturns(containers, nil)
	gu := GardenUtil{
		cli: &cli,
	}

	result, err := gu.ListContainers()
	assert.Nil(t, err)
	assert.Len(t, result, 3)
}
