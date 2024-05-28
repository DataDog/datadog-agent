// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathtest_GetHash(t *testing.T) {
	p1 := Pathtest{
		Hostname: "aaa1",
		Port:     80,
	}
	p2 := Pathtest{
		Hostname: "aaa2",
		Port:     80,
	}
	p3 := Pathtest{
		Hostname: "aaa1",
		Port:     81,
	}

	assert.NotEqual(t, p1.GetHash(), p2.GetHash())
	assert.NotEqual(t, p1.GetHash(), p3.GetHash())
	assert.NotEqual(t, p2.GetHash(), p3.GetHash())
}
