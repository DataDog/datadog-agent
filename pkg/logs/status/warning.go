// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package status

import "sync"

var w = newWarnings()

// Warning is a generic interface that generate warning messages
type Warning interface {
	Render() string
}

// Warnings holds a warning message
type warnings struct {
	raised map[string]Warning
	lock   *sync.Mutex
}

// NewWarnings initialize Warnings with the default values
func newWarnings() *warnings {
	return &warnings{
		raised: make(map[string]Warning),
		lock:   &sync.Mutex{},
	}
}

// Raise opens a RaisedWarning
func Raise(key string, warning Warning) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.raised[key] = warning
}

// GetWarnings returns the message for a key
func GetWarnings() []Warning {
	w.lock.Lock()
	defer w.lock.Unlock()
	warnings := make([]Warning, len(w.raised))
	i := 0
	for _, warning := range w.raised {
		warnings[i] = warning
		i++
	}
	return warnings
}

// Remove marks a RaisedWarning as solved
func Remove(key string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	delete(w.raised, key)
}
