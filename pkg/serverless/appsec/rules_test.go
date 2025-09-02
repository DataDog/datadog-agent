// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"testing"

	"github.com/DataDog/go-libddwaf/v4"
	"github.com/stretchr/testify/require"
)

func TestStaticRule(t *testing.T) {
	if wafHealth() != nil {
		t.Skip("waf disabled")
		return
	}

	builder, err := libddwaf.NewBuilder("", "")
	require.NoError(t, err)
	defer builder.Close()
	_, err = builder.AddDefaultRecommendedRuleset()
	require.NoError(t, err)

	waf := builder.Build()
	require.NoError(t, err)
	waf.Close()
}
