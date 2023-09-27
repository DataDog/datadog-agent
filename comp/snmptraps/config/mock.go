// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package config

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
)

func newMockConfig(tc *TrapsConfig, hnService hostname.Component) (Component, error) {
	host, err := hnService.Get(context.Background())
	if err != nil {
		return nil, err
	}
	if err := tc.SetDefaults(host, "default"); err != nil {
		return nil, err
	}
	return &configService{conf: tc}, nil
}
