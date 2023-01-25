// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package winutil

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows/registry"
)

const (
	AgentRegistryKey    = `SOFTWARE\DataDog\Datadog Agent`
	ClosedSourceKeyName = `AllowClosedSource`
	ClosedSourceAllowed = 1
	ClosedSourceDenied  = 0
)

func GetClosedSourceConsent() (consentVal uint64, err error) {

	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, AgentRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		log.Warnf("unable to open registry key %s: %v", AgentRegistryKey, err)
		return
	}
	defer regKey.Close()

	consentVal, _, err = regKey.GetIntegerValue(ClosedSourceKeyName)
	if err != nil {
		log.Warnf("unable to get value for %s: %v", ClosedSourceKeyName, err)
		return
	}
	return
}

// Determine if closed source is allowed or denied on the host
func IsClosedSourceAllowed() (allowed bool, err error) {
	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, AgentRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		log.Warnf("unable to open registry key %s: %v", AgentRegistryKey, err)
		return
	}
	defer regKey.Close()

	regValue, _, err := regKey.GetIntegerValue(ClosedSourceKeyName)
	if err != nil {
		log.Warnf("unable to get value for %s: %v", ClosedSourceKeyName, err)
		return
	}

	if regValue == ClosedSourceAllowed {
		allowed = true
	}
	return
}

// Allow closed source, if already allowed, does nothing
func AllowClosedSource() error {

	allowed, err := IsClosedSourceAllowed()
	if err != nil {
		return err
	}

	if allowed {
		return nil
	}

	access := registry.SET_VALUE
	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, AgentRegistryKey, uint32(access))
	if err != nil {
		log.Warnf("unable to open registry key %s: %v", AgentRegistryKey, err)
		return err
	}
	defer regKey.Close()

	err = regKey.SetDWordValue(ClosedSourceKeyName, uint32(ClosedSourceAllowed))
	if err != nil {
		return err
	}
	return nil
}

// Deny closed source, if already denied, does nothing
func DenyClosedSource() error {

	allowed, err := IsClosedSourceAllowed()
	if err != nil {
		return err
	}

	if !allowed {
		return nil
	}

	access := registry.SET_VALUE
	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, AgentRegistryKey, uint32(access))
	if err != nil {
		log.Warnf("unable to open registry key %s: %v", AgentRegistryKey, err)
		return err
	}
	defer regKey.Close()

	err = regKey.SetDWordValue(ClosedSourceKeyName, uint32(ClosedSourceDenied))
	if err != nil {
		return err
	}
	return nil

}
