// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package coredump

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Setup enables core dumps and sets the core dump size limit based on configuration.
// Core dumps and go_crash_report are not supported on Windows: go_crash_report is ignored.
func Setup(cfg model.Reader) error {
	if cfg.GetBool("go_crash_report") {
		log.Warn("go_crash_report is not supported on Windows and is ignored")
	}
	if cfg.GetBool("go_core_dump") {
		return errors.New("Not supported on Windows")
	}
	return nil
}
