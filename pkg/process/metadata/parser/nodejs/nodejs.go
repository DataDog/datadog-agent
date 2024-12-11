// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodejsparser wraps functions to guess service name for node applications
package nodejsparser

import (
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FindNameFromNearestPackageJSON finds the package.json walking up from the absFilePath.
// if a package.json is found, returns the value of the field name if declared
func FindNameFromNearestPackageJSON(absFilePath string) (string, bool) {
	current := filepath.Dir(absFilePath)
	up := filepath.Dir(current)
	for run := true; run; run = current != up {
		value, ok := maybeExtractServiceName(filepath.Join(current, "package.json"))
		if ok {
			return value, ok && len(value) > 0
		}
		current = up
		up = path.Dir(current)
	}
	value, ok := maybeExtractServiceName(filepath.Join(current, "package.json")) // this is for the root folder
	return value, ok && len(value) > 0

}

// maybeExtractServiceName return true if a package.json has been found and eventually the value of its name field inside.
func maybeExtractServiceName(filename string) (string, bool) {
	reader, err := os.Open(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Tracef("Error opening package.js file at %s: %v", filename, err)
		}
		return "", false
	}
	defer reader.Close()
	value := doStreamingParse(reader, "name", 0)
	return value, true
}

// doStreamingParse returns the first occurrence of the key at provided level
func doStreamingParse(reader io.Reader, key string, level int) string {
	decoder := json.NewDecoder(reader)

	n := 0
	expect := false
	isKey := false
	t, err := decoder.Token()
	if err != nil || t != json.Delim('{') {
		// expected start of object and no error
		return ""
	}
	for {
		t, err := decoder.Token()
		if err != nil {
			return ""
		}
		tokenRune, ok := t.(rune)
		if ok {
			switch tokenRune {
			case '{', '[':
				n++
				continue
			case '}', ']':
				n--
				continue
			}
		}

		if n == level {
			isKey = !isKey
			if isKey && t == key {
				expect = true
			} else if expect {
				val, _ := t.(string)
				return val
			}
		} else if expect {
			// expected a value, but it's a nested object or array
			return ""
		}
	}
}
