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

// This function retrieves the current consent value from the Winows registry
func GetClosedSourceConsent() (consentVal uint64, err error) {

	// try to open the Datadog agent reg key
	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, AgentRegistryKey, registry.QUERY_VALUE)
	if err != nil { // couldn't open
		log.Warnf("unable to open registry key %s: %v", AgentRegistryKey, err)
		return // consentVal will be 0 (denied)
	}
	defer regKey.Close()

	// get the value for AllowClosedSource
	consentVal, _, err = regKey.GetIntegerValue(ClosedSourceKeyName)
	if err != nil {
		log.Warnf("unable to get value for %s: %v", ClosedSourceKeyName, err)
		return // consentVal will be 0 (denied)
	}
	return
}

// Determine if closed source is allowed or denied on the host
func IsClosedSourceAllowed() (allowed bool, err error) {
	val, err := GetClosedSourceConsent()
	if err != nil {
		return
	}
	return (val == ClosedSourceAllowed), err
}

func SetClosedSourceAllowed(allow bool) (err error) {
	if allow {
		err = allowClosedSource()
	} else {
		err = denyClosedSource()
	}
	return
}

// Allow closed source, if already allowed, does nothing
func allowClosedSource() error {

	// check current value
	allowed, err := IsClosedSourceAllowed()
	if err != nil {
		return err
	}

	// already allowed, return
	if allowed {
		return nil
	}

	// not allowed, so update the value to allowed
	err = doSetKey(ClosedSourceAllowed)
	if err != nil {
		return err
	}
	return nil
}

// Deny closed source, if already denied, does nothing
func denyClosedSource() error {

	// check current value
	allowed, err := IsClosedSourceAllowed()
	if err != nil {
		return err
	}

	// already not allowed, return
	if !allowed {
		return nil
	}

	// allowed, so update the value to denied
	err = doSetKey(ClosedSourceDenied)
	if err != nil {
		return err
	}
	return nil
}

// helper function for opening and setting key value
func doSetKey(val uint32) error {

	// only need to set the value
	access := registry.SET_VALUE

	// try to open the key
	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, AgentRegistryKey, uint32(access))
	if err != nil {
		return err
	}
	defer regKey.Close()

	// try to set to val
	err = regKey.SetDWordValue(ClosedSourceKeyName, val)
	if err != nil {
		return err
	}
	return nil
}
