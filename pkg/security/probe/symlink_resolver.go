// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"os"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type SymlinkResolver struct {
	cache map[unsafe.Pointer]*eval.StringValues
}

func (s *SymlinkResolver) Resolve(key unsafe.Pointer, path ...string) *eval.StringValues {
	if values, exists := s.cache[key]; exists {
		return values
	}

	values := &eval.StringValues{}

	for _, p := range path {
		// re-add existing value
		values.AppendScalarValue(p)

		dest, err := os.Readlink(p)
		if err != nil {
			continue
		}
		//seclog.Tracef("Dispatching custom event %s\n", prettyEvent)
		log.Tracef("Symlink resolved %s => %s\n", p, dest)
		values.AppendScalarValue(dest)
	}

	s.cache[key] = values

	return values
}

func NewSymLinkResolver() *SymlinkResolver {
	return &SymlinkResolver{
		cache: make(map[unsafe.Pointer]*eval.StringValues),
	}
}
