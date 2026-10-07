// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	remoteworkloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// TestRemoteWorkloadmetaParamsLeaveOutImages checks that the agents fed by the
// core agent stream containers and leave out images, which only it reads.
func TestRemoteWorkloadmetaParamsLeaveOutImages(t *testing.T) {
	var params remoteworkloadmeta.Params
	app := fx.New(remoteWorkloadmetaParams(), fx.Populate(&params), fx.NopLogger)
	require.NoError(t, app.Err())

	assert.True(t, params.Filter.MatchKind(workloadmeta.KindContainer))
	assert.False(t, params.Filter.MatchKind(workloadmeta.KindContainerImageMetadata))
}
