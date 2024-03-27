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
	"github.com/spf13/afero"
	"github.com/vibrantbyte/go-antpath/antpath"
)

const (
	// maxParseFileSize is the maximum file size in bytes the parser will accept.
	maxParseFileSize = 1024 * 1024
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

type closeFn func() error

// fileSystemCloser wraps a FileSystem with a Closer in case the filesystem has been created with a stream that
// should be closed after its usage.
type fileSystemCloser struct {
	fs afero.Fs
	cf closeFn
}

func (fsc *fileSystemCloser) Close() error {
	if fsc.cf != nil {
		return fsc.cf()
	}
	return nil
}

// newArgumentSource a PropertyGetter that is taking key=value from the list of arguments provided
// it can be done to parse both java system properties (the prefix is `-D`) or spring boot property args (the prefix is `--`)
func newArgumentSource(arguments []string, prefix string) props.PropertyGetter {
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

// canSafelyParse determines if a file's size is less than the maximum allowed to prevent OOM when parsing.
func canSafelyParse(file afero.File) bool {
	fi, err := file.Stat()
	return err == nil && fi.Size() <= maxParseFileSize
}

// newPropertySourceFromStream create a PropertyGetter by selecting the most appropriate parser giving the file extension.
// An error will be returned if the filesize is greater than maxParseFileSize
func newPropertySourceFromStream(rc io.Reader, filename string, filesize uint64) (props.PropertyGetter, error) {
	if filesize > maxParseFileSize {
		return nil, fmt.Errorf("unable to parse %q. max file size exceeded(actual: %d, max: %d)", filename, filesize, maxParseFileSize)
	}
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
	return properties, err
}

// newPropertySourceFromFile wraps filename opening and closing, delegating the rest of the logic to newPropertySourceFromStream
func newPropertySourceFromFile(filename string) (props.PropertyGetter, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return newPropertySourceFromStream(f, filename, uint64(fi.Size()))
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
			startPath := longestPathPrefix(pattern)
			_ = filepath.WalkDir(filepath.FromSlash(startPath), func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				path := filepath.ToSlash(p)
				if !matcher.MatchStart(pattern, path) {
					if d.IsDir() {
						// skip the whole directory subtree since the prefix does not match
						return filepath.SkipDir
					}
					// skip the current file
					return nil
				}
				// a match is found
				value, _ := newPropertySourceFromFile(path)
				if value != nil {
					arr, ok := ret[profile]
					if !ok {
						arr = &props.Combined{Sources: []props.PropertyGetter{}}
						ret[profile] = arr
					}
					arr.Sources = append(arr.Sources, value)
				}
				return nil
			})
		}
	}
	return ret
}

// extractJavaPropertyFromArgs loops through the command argument to see if a system property declaration matches the provided name.
// name should be in the form of `-D<property_name>=` (`-D` prolog and `=` epilogue) to avoid concatenating strings on each function call.
// The function returns the property value if found and a bool (true if found, false otherwise)
func extractJavaPropertyFromArgs(args []string, name string) (string, bool) {
	for _, a := range args {
		if strings.HasPrefix(a, name) {
			return strings.TrimPrefix(a, name), true
		}
	}
	return "", false
}

// XMLStringToBool parses string element value and return false if explicitly set to `false` or `0`
func XMLStringToBool(s string) bool {
	switch strings.ToLower(s) {
	case "0", "false":
		return false
	}
	return true
}
