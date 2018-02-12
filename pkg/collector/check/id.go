// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package check

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// ID is the representation of the unique ID of a Check instance
type ID string

// Identify returns an unique ID for a check and its configuration
func Identify(check Check, instance ConfigData, initConfig ConfigData) ID {
	return BuildID(check.String(), instance, initConfig)
}

// BuildID returns an unique ID for a check name and its configuration
func BuildID(checkName string, instance, initConfig ConfigData) ID {
	h := fnv.New64()
	h.Write([]byte(instance))
	h.Write([]byte(initConfig))

	id := fmt.Sprintf("%s:%x", checkName, h.Sum64())
	return ID(id)
}

// IDToCheckName returns the check name from a check ID
func IDToCheckName(id ID) string {
	return strings.SplitN(string(id), ":", 2)[0]
}
