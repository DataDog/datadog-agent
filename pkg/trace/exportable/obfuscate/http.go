// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
)

// obfuscateHTTP obfuscates query strings and path segments containing digits in the span's
// "http.url" tag, when one or both of these options are enabled.
func (o *Obfuscator) obfuscateHTTP(span *pb.Span) {
	if span.Meta == nil {
		return
	}
	if !o.opts.HTTP.RemoveQueryString && !o.opts.HTTP.RemovePathDigits {
		// nothing to do
		return
	}
	const k = "http.url"
	val, ok := span.Meta[k]
	if !ok {
		return
	}
	u, err := url.Parse(val)
	if err != nil {
		// should not happen for valid URLs, but better obfuscate everything
		// rather than expose sensitive information when this option is on.
		span.Meta[k] = "?"
		return
	}
	if o.opts.HTTP.RemoveQueryString && u.RawQuery != "" {
		u.ForceQuery = true // add the '?'
		u.RawQuery = ""
	}
	if o.opts.HTTP.RemovePathDigits {
		segs := strings.Split(u.Path, "/")
		var changed bool
		for i, seg := range segs {
			for _, ch := range []byte(seg) {
				if ch >= '0' && ch <= '9' {
					// we can not set the question mark directly here because the url
					// package will escape it into %3F, so we use this placeholder and
					// replace it further down.
					segs[i] = "/REDACTED/"
					changed = true
					break
				}
			}
		}
		if changed {
			u.Path = strings.Join(segs, "/")
		}
	}
	span.Meta[k] = strings.Replace(u.String(), "/REDACTED/", "?", -1)
}
