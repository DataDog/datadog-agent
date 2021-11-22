// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || linux_bpf
// +build windows linux_bpf

package classifier

// NewNullClassifier returns a dummy implementation of Classifier
func NewNullClassifier() Classifier {
	return nullClassifier{}
}

type nullClassifier struct{}

func (nullClassifier) GetStats() map[string]int64 {
	return map[string]int64{
		"packets_received": 0,
	}
}
func (nullClassifier) DumpMaps(maps ...string) (string, error) { return "", nil }

func (nullClassifier) Close() {}

var _ Classifier = nullClassifier{}
