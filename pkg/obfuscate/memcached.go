// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"fmt"
	"strings"
)

// ObfuscateMemcachedString obfuscates the Memcached command cmd.
func (o *Obfuscator) ObfuscateMemcachedString(cmd string) string {
	// All memcached commands end with new lines [1]. In the case of storage
	// commands, key values follow after. Knowing this, all we have to do
	// to obfuscate the values is to remove everything that follows
	// a new line. For non-storage commands, this will have no effect.
	// [1]: https://github.com/memcached/memcached/blob/master/doc/protocol.txt
	ret := strings.SplitN(cmd, "\r\n", 2)[0]
	ret = strings.TrimSpace(ret)
	if !o.opts.Memcached.RemoveKey {
		return ret
	}
	// Remove the <key> from the command, assuming it's in the form
	// "<method> <key> <flag> <ttl> <cas>"
	spaces := strings.SplitN(ret, " ", 3)
	if len(spaces) == 1 {
		return ret
	}
	if len(spaces) == 2 {
		return fmt.Sprintf("%v ?", spaces[0])
	}
	return fmt.Sprintf("%v ? %v", spaces[0], spaces[2])
}
