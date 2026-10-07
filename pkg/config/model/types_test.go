// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import "testing"

func TestSourceProductEnablementPriority(t *testing.T) {
	greater := [][2]Source{
		{SourceProductEnablement, SourceUnknown},
		{SourceProductEnablement, SourceDefault},
		{SourceInfraMode, SourceProductEnablement},
		{SourceFile, SourceProductEnablement},
	}
	for _, pair := range greater {
		if !pair[0].IsGreaterThan(pair[1]) {
			t.Errorf("expected %s to be greater than %s", pair[0], pair[1])
		}
	}

	if previous := SourceProductEnablement.PreviousSource(); previous != SourceUnknown {
		t.Errorf("expected previous source of %s to be %s, got %s", SourceProductEnablement, SourceUnknown, previous)
	}
	if previous := SourceInfraMode.PreviousSource(); previous != SourceProductEnablement {
		t.Errorf("expected previous source of %s to be %s, got %s", SourceInfraMode, SourceProductEnablement, previous)
	}
}

// PreviousSource indexes Sources by priority, so both must stay in sync.
func TestSourcesMatchPriorities(t *testing.T) {
	// +1 for SourceSchema, which has a priority but isn't part of Sources
	if len(sourcesPriority) != len(Sources)+1 {
		t.Errorf("expected %d priorities, got %d", len(Sources)+1, len(sourcesPriority))
	}
	for index, source := range Sources {
		if sourcesPriority[source] != index {
			t.Errorf("source %s: expected priority %d, got %d", source, index, sourcesPriority[source])
		}
	}
}
