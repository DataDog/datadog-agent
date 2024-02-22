// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package runner

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

var _ Profile = &mockProfile{}

func newMockProfile(storeMap map[parameters.StoreKey]string) Profile {
	store := parameters.NewMockStore(storeMap)
	mp := mockProfile{baseProfile: newProfile("totoro", []string{}, store, nil, "")}
	mp.baseProfile.workspaceRootFolder = "mock"
	return mp
}

type mockProfile struct {
	baseProfile
}

// NamePrefix returns a prefix to name objects
func (mp mockProfile) NamePrefix() string {
	return "mock"
}

// AllowDevMode returns if DevMode is allowed
func (mp mockProfile) AllowDevMode() bool {
	return true
}
