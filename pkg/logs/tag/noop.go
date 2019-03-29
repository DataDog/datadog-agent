// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tag

type noop struct {
	tags []string
}

// GetTags returns an empty list of tags.
func (p *noop) GetTags() []string {
	return p.tags
}

// Start does nothing
func (p *noop) Start() {}

// Stop does nothing
func (p *noop) Stop() {}
