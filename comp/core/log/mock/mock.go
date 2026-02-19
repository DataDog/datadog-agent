// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mock

import (
	"strings"
	"testing"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// tbWriter is an implementation of io.Writer that sends lines to
// testing.TB#Log.
type tbWriter struct {
	t testing.TB
}

// Write implements Writer#Write.
func (tbw *tbWriter) Write(p []byte) (n int, err error) {
	// this assumes that seelog always writes one log entry in one Write call
	msg := strings.TrimSuffix(string(p), "\n")
	tbw.t.Log(msg)
	return len(p), nil
}

// New returns a new mock for the log Component
func New(t testing.TB) log.Component {
	// Build a logger that only logs to t.Log(..)
	iface, err := pkglog.LoggerFromWriterWithMinLevelAndFullFormat(&tbWriter{t}, pkglog.TraceLvl)
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Cleanup(func() {
		// stop using the logger to avoid a race condition
		pkglog.SetupLogger(pkglog.Default(), pkglog.DebugStr)
		iface.Close()
	})

	// install the logger into pkg/util/log
	pkglog.SetupLogger(iface, pkglog.DebugStr)

	return pkglog.NewWrapper(2)
}
