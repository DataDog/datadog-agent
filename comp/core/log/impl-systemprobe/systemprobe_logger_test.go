// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	comp "github.com/DataDog/datadog-agent/comp/def"
)

func TestNewComponent(t *testing.T) {
	testLC := comp.NewTestLifecycle(t)
	deps := Requires{
		Lc:     testLC,
		Params: log.ForOneShot("test", "info", false),
		Config: sysprobeconfigimpl.NewMock(t),
	}

	provides, err := NewComponent(deps)
	require.NoError(t, err)
	assert.NotNil(t, provides.Comp)
	testLC.AssertHooksNumber(1)
}
