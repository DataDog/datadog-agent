// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/simplelru"
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

// Important should always be called after having checked that the file is not a discarder itself otherwise it can report incorrect
// parent discarder
func isParentPathDiscarder(rs *rules.RuleSet, regexCache *simplelru.LRU, eventType EventType, filenameField eval.Field, filename string) (bool, error) {
	dirname := filepath.Dir(filename)

	bucket := rs.GetBucket(eventType.String())
	if bucket == nil {
		return false, nil
	}

	basenameField := strings.Replace(filenameField, ".filename", ".basename", 1)

	event := NewEvent(nil)
	if _, err := event.GetFieldType(filenameField); err != nil {
		return false, nil
	}

	if _, err := event.GetFieldType(basenameField); err != nil {
		return false, nil
	}

	for _, rule := range bucket.GetRules() {
		// ensure we don't push parent discarder if there is another rule relying on the parent path

		// first case: rule contains a filename field
		// ex: rule		open.filename == "/etc/passwd"
		//     discarder /etc/fstab
		// /etc/fstab is a discarder but not the parent

		// second case: rule doesn't contain a filename field but a basename field
		// ex: rule	 	open.basename == "conf.d"
		//     discarder /etc/conf.d/httpd.conf
		// /etc/conf.d/httpd.conf is a discarder but not the parent

		// check filename
		if values := rule.GetFieldValues(filenameField); len(values) > 0 {
			for _, value := range values {
				if value.Type == eval.PatternValueType {
					if value.Regex.MatchString(dirname) {
						return false, nil
					}

					valueDir := path.Dir(value.Value.(string))
					var regexDir *regexp.Regexp
					if entry, found := regexCache.Get(valueDir); found {
						regexDir = entry.(*regexp.Regexp)
					} else {
						var err error
						regexDir, err = regexp.Compile(valueDir)
						if err != nil {
							return false, err
						}
						regexCache.Add(valueDir, regexDir)
					}

					if regexDir.MatchString(dirname) {
						return false, nil
					}
				} else {
					if strings.HasPrefix(value.Value.(string), dirname) {
						return false, nil
					}
				}
			}

			if err := event.SetFieldValue(filenameField, dirname); err != nil {
				return false, err
			}

			if isDiscarder, _ := rs.IsDiscarder(event, filenameField); isDiscarder {
				return true, nil
			}
		}

		// check basename
		if values := rule.GetFieldValues(basenameField); len(values) > 0 {
			if err := event.SetFieldValue(basenameField, path.Base(dirname)); err != nil {
				return false, err
			}

			if isDiscarder, _ := rs.IsDiscarder(event, basenameField); !isDiscarder {
				return false, nil
			}
		}
	}

	log.Tracef("`%s` discovered as parent discarder", dirname)

	return true, nil
}
