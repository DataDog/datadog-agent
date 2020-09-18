// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

// NewResolvers creates a new instance of Resolvers
func NewResolvers(probe *Probe) (*Resolvers, error) {
	dentryResolver, err := NewDentryResolver(probe)
	if err != nil {
		return nil, err
	}
	timeResolver, err := NewTimeResolver()
	if err != nil {
		return nil, err
	}
	return &Resolvers{
		probe:          probe,
		DentryResolver: dentryResolver,
		MountResolver:  NewMountResolver(probe),
		TimeResolver:   timeResolver,
	}, nil
}
