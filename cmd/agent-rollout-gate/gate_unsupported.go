// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package main

import (
	"errors"
	"io"
)

func waitAndExec(options, io.Writer) error {
	return errors.New("component lock handoff is supported only on Linux")
}
