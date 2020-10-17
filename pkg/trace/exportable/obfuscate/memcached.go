// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package obfuscate

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
)

func (*Obfuscator) obfuscateMemcached(span *pb.Span) {
	const k = "memcached.command"
	if span.Meta == nil || span.Meta[k] == "" {
		return
	}
	// All memcached commands end with new lines [1]. In the case of storage
	// commands, key values follow after. Knowing this, all we have to do
	// to obfuscate sensitive information is to remove everything that follows
	// a new line. For non-storage commands, this will have no effect.
	// [1]: https://github.com/memcached/memcached/blob/master/doc/protocol.txt
	cmd := strings.SplitN(span.Meta[k], "\r\n", 2)[0]
	span.Meta[k] = strings.TrimSpace(cmd)
}
