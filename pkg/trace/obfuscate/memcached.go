// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/traces"
)

func (*Obfuscator) obfuscateMemcached(span traces.Span) {
	const k = "memcached.command"

	v, ok := span.GetMetaUnsafe(k)
	if !ok || v == "" {
		return
	}

	// All memcached commands end with new lines [1]. In the case of storage
	// commands, key values follow after. Knowing this, all we have to do
	// to obfuscate sensitive information is to remove everything that follows
	// a new line. For non-storage commands, this will have no effect.
	// [1]: https://github.com/memcached/memcached/blob/master/doc/protocol.txt
	cmd := strings.SplitN(v, "\r\n", 2)[0]
	span.SetMeta(k, strings.TrimSpace(cmd))
}
