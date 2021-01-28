// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

//+build !zlib

package stream

import (
	"errors"
)

const (
	// Available is true if the code is compiled in
	Available = false
)

var (
	// ErrItemTooBig is returned when a item alone exceeds maximum payload size
	ErrItemTooBig = errors.New("item alone exceeds maximum payload size")
)
