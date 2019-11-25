// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v5

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {
	pl := GetPayload(util.HostnameData{Hostname: "testhostname", Provider: ""})
	assert.NotNil(t, pl)
}
