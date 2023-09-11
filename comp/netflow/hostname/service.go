// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostname

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

type hostnameService struct {
	name string
}

var _ Component = (*hostnameService)(nil)

// Get returns the hostname.
func (hs *hostnameService) Get() string {
	return hs.name
}

// newHostnameService fetches the hostname and returns a service wrapping it
func newHostnameService() (Component, error) {
	name, err := hostname.Get(context.Background())
	if err != nil {
		return nil, err
	}
	return &hostnameService{
		name: name,
	}, nil
}
