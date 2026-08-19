// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"fmt"
	"testing"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/require"
)

type captureComponent struct{ messages []string }

func (c *captureComponent) Trace(args ...interface{}) {
	c.messages = append(c.messages, fmt.Sprint(args...))
}
func (c *captureComponent) Tracef(format string, args ...interface{}) {
	c.messages = append(c.messages, fmt.Sprintf(format, args...))
}
func (c *captureComponent) Debug(args ...interface{}) {
	c.messages = append(c.messages, fmt.Sprint(args...))
}
func (c *captureComponent) Debugf(format string, args ...interface{}) {
	c.messages = append(c.messages, fmt.Sprintf(format, args...))
}
func (c *captureComponent) Info(args ...interface{}) {
	c.messages = append(c.messages, fmt.Sprint(args...))
}
func (c *captureComponent) Infof(format string, args ...interface{}) {
	c.messages = append(c.messages, fmt.Sprintf(format, args...))
}
func (c *captureComponent) Warn(args ...interface{}) error {
	c.messages = append(c.messages, fmt.Sprint(args...))
	return nil
}
func (c *captureComponent) Warnf(format string, args ...interface{}) error {
	c.messages = append(c.messages, fmt.Sprintf(format, args...))
	return nil
}
func (c *captureComponent) Error(args ...interface{}) error {
	c.messages = append(c.messages, fmt.Sprint(args...))
	return nil
}
func (c *captureComponent) Errorf(format string, args ...interface{}) error {
	c.messages = append(c.messages, fmt.Sprintf(format, args...))
	return nil
}
func (c *captureComponent) Critical(args ...interface{}) error {
	c.messages = append(c.messages, fmt.Sprint(args...))
	return nil
}
func (c *captureComponent) Criticalf(format string, args ...interface{}) error {
	c.messages = append(c.messages, fmt.Sprintf(format, args...))
	return nil
}
func (*captureComponent) Flush() {}

func TestGlobalLoggerPrefixesFormattedAndUnformattedMessages(t *testing.T) {
	t.Cleanup(func() { pkglog.SetLogObserver(nil) })

	var messages []string
	pkglog.SetLogObserver(func(_ pkglog.LogLevel, message string) {
		messages = append(messages, message)
	})

	Infof("configured %d detectors", 2)
	Warn("logssource unavailable")

	require.Equal(t, []string{
		"[anomalydetection] configured 2 detectors",
		"[anomalydetection] logssource unavailable",
	}, messages)
}

func TestWrappedComponentPrefixesFormattedAndUnformattedMessages(t *testing.T) {
	capture := &captureComponent{}
	logger := Wrap(capture)

	logger.Infof("reporter sent %d events", 3)
	require.NoError(t, logger.Warn("observer disabled"))

	require.Equal(t, []string{
		"[anomalydetection] reporter sent 3 events",
		"[anomalydetection] observer disabled",
	}, capture.messages)
}
