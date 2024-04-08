// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package protocols

import (
	"fmt"
	"github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

// GetMap retrieves an eBPF map by name from the provided manager
func GetMap(mgr *manager.Manager, name string) (*ebpf.Map, error) {
	m, _, err := mgr.GetMap(name)
	if err != nil {
		return nil, fmt.Errorf("error getting %q map: %s", name, err)
	}
	if m == nil {
		return nil, fmt.Errorf("%q map is nil", name)
	}
	return m, nil
}
