// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fakeintake owns the fakeintake receiver selection schema.
package fakeintake

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

type Config struct {
	RemoteConfig string `yaml:"remote-config" default:"disabled" enum:"disabled,receiver" description:"RC policy. Receiver RC is not implemented in managed installs yet."`
}
type Rules struct{}

func (Rules) Validate(c Config) error {
	if c.RemoteConfig != "disabled" {
		return fmt.Errorf("remote-config: receiver RC is not implemented; use disabled")
	}
	return nil
}

var Schema = configschema.Must[Config](Rules{})
