// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/appsec-internal-go/appsec"
	waf "github.com/DataDog/go-libddwaf"

	"github.com/stretchr/testify/require"
)

func TestStaticRule(t *testing.T) {
	if wafHealth() != nil {
		t.Skip("waf disabled")
		return
	}

	var rules map[string]any
	err := json.Unmarshal([]byte(appsec.StaticRecommendedRules), &rules)
	require.NoError(t, err)

	waf, err := waf.NewHandle(rules, "", "")
	require.NoError(t, err)
	waf.Close()
}
