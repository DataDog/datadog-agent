// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodejsparser wraps functions to guess service name for node applications
package nodejsparser

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type packageName struct {
	// Name of the package in the json file
	Name string `json:"name,omitempty"`
}

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
	pn := packageName{}
	err = json.NewDecoder(reader).Decode(&pn)
	if err != nil {
		log.Tracef("Error decoding package.js file at %s: %v", filename, err)
	}
	return pn.Name, true
}
