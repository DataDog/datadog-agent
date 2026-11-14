// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hosttags

// Getter returns the host tags to attach to a payload.
type Getter interface {
	GetHostTags() []string
}

type withExtraTags struct {
	base  Getter
	extra func() []string
}

// WithExtraTags returns a Getter that adds the tags returned by extra to the host tags of base.
// Unlike the expected host tags of a HostTagProvider, the extra tags don't expire: extra is
// called on every GetHostTags call.
func WithExtraTags(base Getter, extra func() []string) Getter {
	return &withExtraTags{base: base, extra: extra}
}

// GetHostTags returns the host tags of the base Getter followed by the extra tags.
func (w *withExtraTags) GetHostTags() []string {
	hostTags := w.base.GetHostTags()
	extra := w.extra()
	if len(extra) == 0 {
		return hostTags
	}
	if len(hostTags) == 0 {
		return extra
	}
	// copy so we never append to the base Getter's slice
	tags := make([]string, 0, len(hostTags)+len(extra))
	tags = append(tags, hostTags...)
	return append(tags, extra...)
}
