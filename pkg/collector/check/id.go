// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package check

import (
	"hash/fnv"
	"strconv"
)

// ID is the representation of the unique ID of a Check instance
type ID string

// Identify returns the ID of the check
func Identify(check Check, instance ConfigData, initConfig ConfigData) ID {
	h := fnv.New64()
	h.Write([]byte(instance))
	h.Write([]byte(initConfig))

	id := check.String() + ":"
	id += strconv.FormatUint(h.Sum64(), 16)
	return ID(id)
}
