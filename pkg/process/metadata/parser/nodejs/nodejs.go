// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodejs wraps functions to guess service name for node applications
package nodejs

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
	"path"
	"path/filepath"
)

type packageName struct {
	// Name of the package in the json file
	Name string `json:"name,omitempty"`
}

// FindNameFromNearestPackageJSON finds the package.json walking up from the absFilePath.
// if a package.json is found, returns the value of the field name if declared
func FindNameFromNearestPackageJSON(absFilePath string) (bool, string) {
	current := filepath.Dir(absFilePath)
	up := filepath.Dir(current)
	for run := true; run; run = current != up {
		ok, value := maybeExtractServiceName(filepath.Join(current, "package.json"))
		if ok {
			return ok && len(value) > 0, value
		}
		current = up
		up = path.Dir(current)
	}
	ok, value := maybeExtractServiceName(filepath.Join(current, "package.json")) // this is for the root folder
	return ok && len(value) > 0, value

}

// maybeExtractServiceName return true if a package.json has been found and eventually the value of its name field inside.
func maybeExtractServiceName(filename string) (bool, string) {
	// we can use a reader and use a json decoder also to limit the memory used and generally speaking to be safe.
	// However, package.json files are supposed to be small and Unmarshall offers validation
	bytes, err := os.ReadFile(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Tracef("Error opening package.js file at %s: %v", filename, err)
		}
		return false, ""
	}
	pn := packageName{}
	err = json.Unmarshal(bytes, &pn)
	if err != nil {
		log.Tracef("Error decoding package.js file at %s: %v", filename, err)
	}
	return true, pn.Name
}
