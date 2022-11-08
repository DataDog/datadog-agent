// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf"
)

func TestStaticRule(t *testing.T) {
	if waf.Health() != nil {
		t.Skip("waf disabled")
		return
	}
	waf, err := waf.NewHandle([]byte(staticRecommendedRule), "", "")
	require.NoError(t, err)
	waf.Close()
}
