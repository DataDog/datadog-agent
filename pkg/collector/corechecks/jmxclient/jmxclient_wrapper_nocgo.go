// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo

package jmxclient

import (
	"errors"
	"unsafe"
)

// JmxClientWrapper wraps the CGo calls to the JmxClient library
type JmxClientWrapper struct {
	isolateThread unsafe.Pointer
}

// NewJmxClientWrapper creates a new wrapper instance
func NewJmxClientWrapper(isolateThread unsafe.Pointer) *JmxClientWrapper {
	return &JmxClientWrapper{
		isolateThread: isolateThread,
	}
}

// ConnectJVM is not available without CGo
func (w *JmxClientWrapper) ConnectJVM(host string, port int) (int, error) {
	return 0, errors.New("jmxclient check requires CGo support")
}

// PrepareBeans is not available without CGo
func (w *JmxClientWrapper) PrepareBeans(sessionID int, beansConfig string) error {
	return errors.New("jmxclient check requires CGo support")
}

// CollectBeans is not available without CGo
func (w *JmxClientWrapper) CollectBeans(sessionID int) (string, error) {
	return "", errors.New("jmxclient check requires CGo support")
}

// BeanAttribute represents a single attribute name-value pair
type BeanAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BeanData represents a collected JMX bean with its attributes
type BeanData struct {
	Path       string          `json:"path"`
	Success    bool            `json:"success"`
	Attributes []BeanAttribute `json:"attributes"`
	Attribute  string          `json:"attribute"`
	Type       string          `json:"type"`
}

// CollectBeansAsMap is not available without CGo
func (w *JmxClientWrapper) CollectBeansAsMap(sessionID int) (map[string]interface{}, error) {
	return nil, errors.New("jmxclient check requires CGo support")
}

// CollectBeansAsStructs is not available without CGo
func (w *JmxClientWrapper) CollectBeansAsStructs(sessionID int) ([]BeanData, error) {
	return nil, errors.New("jmxclient check requires CGo support")
}

// CloseJVM is not available without CGo
func (w *JmxClientWrapper) CloseJVM(sessionID int) error {
	return errors.New("jmxclient check requires CGo support")
}

// CleanupAll is not available without CGo
func (w *JmxClientWrapper) CleanupAll() error {
	return errors.New("jmxclient check requires CGo support")
}
