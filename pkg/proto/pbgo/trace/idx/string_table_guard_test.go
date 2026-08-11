// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// allowedStringFields lists the fully-qualified proto field names that are permitted
// to use a raw string type. Only the string table itself is allowed: every other
// string value reachable from the idx TracerPayload must be stored as a uint32
// reference into this table so identical strings are deduplicated across the payload.
var allowedStringFields = map[protoreflect.FullName]struct{}{
	"datadog.trace.idx.TracerPayload.strings": {},
}

// TestNoRawStringFields guards against regressions where a new raw string field is
// added to the idx TracerPayload or any message reachable from it (chunks, spans,
// attributes, container debug info, etc.). Raw strings bloat the payload because
// identical values are serialized repeatedly instead of being deduplicated through
// the string table. New string values must be stored as a uint32 string-table
// reference (see the various *Ref fields) rather than a raw string.
//
// If this test fails, do not add the offending field to the allowlist: convert it to
// a uint32 reference into the strings table instead.
func TestNoRawStringFields(t *testing.T) {
	visited := make(map[protoreflect.FullName]struct{})

	var walk func(md protoreflect.MessageDescriptor)
	walk = func(md protoreflect.MessageDescriptor) {
		if _, seen := visited[md.FullName()]; seen {
			return
		}
		visited[md.FullName()] = struct{}{}

		fields := md.Fields()
		for i := 0; i < fields.Len(); i++ {
			f := fields.Get(i)
			if _, ok := allowedStringFields[f.FullName()]; ok {
				continue
			}

			if f.IsMap() {
				if f.MapKey().Kind() == protoreflect.StringKind {
					t.Errorf("map field %s has a string key; use a uint32 string-table reference instead", f.FullName())
				}
				if mv := f.MapValue(); mv.Kind() == protoreflect.StringKind {
					t.Errorf("map field %s has a string value; use a uint32 string-table reference instead", f.FullName())
				} else if msg := mv.Message(); msg != nil {
					walk(msg)
				}
				continue
			}

			// Covers singular, repeated (e.g. repeated string), and oneof string fields.
			if f.Kind() == protoreflect.StringKind {
				t.Errorf("field %s is a raw string; use a uint32 string-table reference instead (see the *Ref fields)", f.FullName())
				continue
			}

			if msg := f.Message(); msg != nil {
				walk(msg)
			}
		}
	}

	walk((&TracerPayload{}).ProtoReflect().Descriptor())
}
