// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package configimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	trapsconf "github.com/DataDog/datadog-agent/comp/snmptraps/config/def"
)

// NewMockConfig creates a config component for use in tests. It sets sensible
// defaults on the provided TrapsConfig using the hostname service.
func NewMockConfig(hn hostname.Component, conf *trapsconf.TrapsConfig) (trapsconf.Component, error) {
	host, err := hn.Get(context.Background())
	if err != nil {
		return nil, err
	}
	if err := conf.SetDefaults(host, "default"); err != nil {
		return nil, err
	}
	return &configService{conf: conf}, nil
}
