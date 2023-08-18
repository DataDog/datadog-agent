// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"testing"

	"github.com/DataDog/appsec-internal-go/appsec"
	"github.com/DataDog/go-libddwaf"

	"github.com/stretchr/testify/require"
)

func TestStaticRule(t *testing.T) {
	if waf.Health() != nil {
		t.Skip("waf disabled")
		return
	}
	waf, err := waf.NewHandle([]byte(appsec.StaticRecommendedRules), "", "")
	require.NoError(t, err)
	waf.Close()
}
