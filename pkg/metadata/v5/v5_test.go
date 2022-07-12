// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v5

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {
	ctx := context.Background()
	pl := GetPayload(ctx, hostname.Data{Hostname: "testhostname", Provider: ""})
	assert.NotNil(t, pl)
}
