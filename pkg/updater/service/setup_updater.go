// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import "github.com/DataDog/datadog-agent/pkg/util/log"

const updaterUnit = "datadog-updater.service"

// todo: make this independent of agent bootstrap
func setupUpdater() (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup updater: %s", err)
			RemoveUpdaterUnits()
		}
	}()
	if err = loadUnit(updaterUnit); err != nil {
		return
	}
	if err = systemdReload(); err != nil {
		return
	}
	if err = enableUnit(updaterUnit); err != nil {
		return
	}
	if err = startUnit(updaterUnit); err != nil {
		return
	}
	return nil
}

// RemoveUpdaterUnits stops and removes updater units
func RemoveUpdaterUnits() {
	if err := stopUnit(updaterUnit); err != nil {
		log.Warnf("Failed to stop %s: %s", updaterUnit, err)
	}
	if err := disableUnit(updaterUnit); err != nil {
		log.Warnf("Failed to disable %s: %s", updaterUnit, err)
	}
	if err := removeUnit(updaterUnit); err != nil {
		log.Warnf("Failed to remove %s: %s", updaterUnit, err)
	}
}
