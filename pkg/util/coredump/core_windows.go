// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package coredump

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Setup enables core dumps and sets the core dump size limit based on configuration
func Setup(cfg model.Reader) error {
	if cfg.GetBool("go_core_dump") {
		return errors.New("Not supported on Windows")
	}
	return nil
}
