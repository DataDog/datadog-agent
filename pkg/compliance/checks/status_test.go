// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	assert "github.com/stretchr/testify/require"
)

func TestStatus(t *testing.T) {

	status := newStatus()

	status.addCheck(&compliance.CheckStatus{
		RuleID:    "rule-1",
		Name:      "rule one",
		Framework: "framework",
		Source:    "source",
		Version:   "version",
		InitError: errors.New("failed to initialize"),
	})

	status.addCheck(&compliance.CheckStatus{
		RuleID:    "rule-2",
		Name:      "rule two",
		Framework: "framework",
		Source:    "source",
		Version:   "version",
	})

	status.updateCheck("rule-2", &event.Event{
		Result: "passed",
	})

	status.updateCheck("rule-3", &event.Event{
		Result: "passed",
	})

	assert.Equal(
		t,
		compliance.CheckStatusList{
			&compliance.CheckStatus{
				RuleID:    "rule-1",
				Name:      "rule one",
				Framework: "framework",
				Source:    "source",
				Version:   "version",
				InitError: errors.New("failed to initialize"),
			},
			&compliance.CheckStatus{
				RuleID:    "rule-2",
				Name:      "rule two",
				Framework: "framework",
				Source:    "source",
				Version:   "version",
				LastEvent: &event.Event{
					Result: "passed",
				},
			},
		},
		status.getChecksStatus(),
	)

}
