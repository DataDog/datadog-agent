// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !jmx

package inventorychecksimpl

func (ic *inventorychecksImpl) getJMXChecksMetadata() (jmxMetadata map[string][]metadata) {
	// This function is a no-op when JMX is not enabled.
	return map[string][]metadata{}
}
