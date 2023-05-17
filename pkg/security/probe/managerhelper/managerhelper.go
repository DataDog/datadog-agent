// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package managerhelper

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
)

// Map returns a map by its name
func Map(manager *manager.Manager, name string) (*lib.Map, error) {
	if manager == nil {
		return nil, fmt.Errorf("failed to get map '%s', manager is null", name)
	}
	m, ok, err := manager.GetMap(name)
	if err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("failed to get map '%s'", name)
	}
	return m, nil
}
