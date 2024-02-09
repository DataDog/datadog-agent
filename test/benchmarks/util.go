// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandomString generates a random string of the given size
func RandomString(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

// TimeNowNano returns Unix time with nanosecond precision
func TimeNowNano() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second)
}

// InitLogging inits default logger
func InitLogging(level string) error {
	err := config.SetupLogger(config.LoggerName("test"), level, "", "", false, true, false)
	if err != nil {
		return fmt.Errorf("Unable to initiate logger: %s", err)
	}

	return nil
}

// SetHostname sets the hostname
func SetHostname(hostname string) {
	mockConfig := config.Mock(nil)
	mockConfig.SetWithoutSource("hostname", hostname)
}
