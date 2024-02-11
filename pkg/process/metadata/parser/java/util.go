// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package javaparser

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rickar/props"
	"github.com/vibrantbyte/go-antpath/antpath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// mapSource is a type holding properties stored as map. It implements PropertyGetter
type mapSource struct {
	m map[string]string
}

// Get the value for the supplied key
func (y *mapSource) Get(key string) (string, bool) {
	val, ok := y.m[key]
	return val, ok
}

// GetDefault gets the value for the supplied key or the defVal if missing
func (y *mapSource) GetDefault(key string, defVal string) string {
	val, ok := y.m[key]
	if !ok {
		return defVal
	}
	return val
}

// newArgumentSource a PropertyGetter that is taking key=value from the list of arguments provided
// it can be done to parse both java system properties (the prefix is `-D`) or spring boot property args (the prefix is `--`)
func newArgumentSource(arguments []string, prefix string) *mapSource {
	parsed := make(map[string]string)
	for _, val := range arguments {
		if !strings.HasPrefix(val, prefix) {
			continue
		}
		parts := strings.SplitN(val[len(prefix):], "=", 2)
		if len(parts) == 1 {
			parsed[parts[0]] = ""
		} else {
			parsed[parts[0]] = parts[1]
		}
	}
	return &mapSource{parsed}
}

// newPropertySourceFromStream create a PropertyGetter by selecting the most appropriate parser giving the file extension.
func newPropertySourceFromStream(rc io.Reader, filename string) (*props.PropertyGetter, error) {
	var properties props.PropertyGetter
	var err error
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".properties":
		properties, err = props.Read(rc)
	case ".yaml", ".yml":
		properties, err = newYamlSource(rc)
	default:
		return nil, fmt.Errorf("unhandled file type for %q", filename)
	}
	return &properties, err
}

// longestPathPrefix extracts the longest path's portion that's not a pattern (i.e. /test/**/*.xml will return /test/)
func longestPathPrefix(pattern string) string {
	idx := strings.IndexAny(pattern, "?*")
	if idx < 0 {
		return pattern
	}
	idx = strings.LastIndex(pattern[:idx], "/")
	if idx < 0 {
		return ""
	}
	return pattern[:idx]
}

// scanSourcesFromFileSystem returns all the PropertyGetter sources built from files matching profilePatterns.
// profilePatterns is a map that has for key the name of the spring profile and for key the values of patterns to be evaluated to find those files
func scanSourcesFromFileSystem(profilePatterns map[string][]string) map[string]*props.Combined {
	ret := make(map[string]*props.Combined)
	matcher := antpath.New()
	for profile, pp := range profilePatterns {
		for _, pattern := range pp {
			path := longestPathPrefix(pattern)
			_ = filepath.WalkDir(filepath.FromSlash(path), func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				path := filepath.ToSlash(p)

				if d.IsDir() {
					if !matcher.MatchStart(pattern, path) {
						return filepath.SkipDir
					}
				} else if matcher.Match(pattern, path) {
					// found
					f, err2 := os.Open(path)
					if err2 != nil {
						log.Tracef("Error while reading properties from %s: %v", path, err2)
						return nil
					}
					var value *props.PropertyGetter
					value, _ = func() (*props.PropertyGetter, error) {
						defer f.Close()
						return newPropertySourceFromStream(f, p)
					}()
					if value != nil {
						arr, ok := ret[profile]
						if !ok {
							arr = &props.Combined{Sources: []props.PropertyGetter{}}
							ret[profile] = arr
						}
						arr.Sources = append(arr.Sources, *value)
					}
				}
				return nil
			})
		}
	}
	return ret
}
