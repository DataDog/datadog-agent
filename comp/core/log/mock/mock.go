// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock offers a mock for the logger.
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
	iface, err := pkglog.LoggerFromWriterWithMinLevelAndFormat(&tbWriter{t}, pkglog.TraceLvl, pkglog.TemplateFormatter("{{.Date \"2006-01-02 15:04:05 MST\"}} | {{.Level}} | ({{.ShortFile}}:{{.line}} in {{.FuncShort}}) | {{.ExtraTextContext}}{{.msg}}\n"))
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Cleanup(func() {
		// stop using the logger to avoid a race condition
		pkglog.SetupLogger(pkglog.Default(), "debug")
	})

	// install the logger into pkg/util/log
	pkglog.SetupLogger(iface, "debug")

	return pkglog.NewWrapper(2)
}
