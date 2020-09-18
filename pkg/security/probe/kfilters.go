// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrDiscarderNotSupported is returned when trying to discover a discarder on a field that doesn't support them
type ErrDiscarderNotSupported struct {
	Field string
}

func (e ErrDiscarderNotSupported) Error() string {
	return fmt.Sprintf("discarder not supported for `%s`", e.Field)
}

// FilterPolicy describes a filtering policy
type FilterPolicy struct {
	Mode  PolicyMode
	Flags PolicyFlag
}

// Bytes returns the binary representation of a FilterPolicy
func (f *FilterPolicy) Bytes() ([]byte, error) {
	return []byte{uint8(f.Mode), uint8(f.Flags)}, nil
}

func isParentPathDiscarder(rs *rules.RuleSet, eventType eval.EventType, filename string) (bool, error) {
	dirname := filepath.Dir(filename)

	// ensure we don't push parent discarder if there is another rule relying on the parent path
	// ex: rule      open.filename == "/etc/passwd"
	//     discarder /etc/fstab
	// /etc/fstab is a discarder but not the parent
	re, err := regexp.Compile("^" + dirname + "/.*$")
	if err != nil {
		return false, err
	}

	values := rs.GetFieldValues(eventType + ".filename")
	for _, value := range values {
		if re.MatchString(value.Value.(string)) {
			return false, nil
		}
	}

	// check basename, assuming there is a basename field
	// ensure that we don't discard a parent that matches a basename rule
	// ex: rule     open.basename == ".ssh"
	//     discader /root/.ssh/id_rsa
	// we can't discard /root/.ssh as basename rule matches it
	// Note: This shouldn't happen we can't have a discarder working on multiple fields.
	//       ex: open.filename == "/etc/passwd"
	//           open.basename == "shadow"
	//       These rules won't return any discarder
	var isDiscarder bool

	field := eventType + ".basename"
	if values := rs.GetFieldValues(field); len(values) == 0 {
		isDiscarder = true
	} else {
		isDiscarder, err = rs.IsDiscarder(field, path.Base(dirname))
		if err != nil {
			if _, ok := err.(*eval.ErrFieldNotFound); ok {
				// no basename rule so we can discard
				isDiscarder = true
			}
		}
	}

	if isDiscarder {
		log.Tracef("`%s` discovered as parent discarder", dirname)
	}

	return isDiscarder, nil
}
