// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"fmt"
	"reflect"
	"strings"
)

// gets a value from a map[string]interface{} using a dot-separated key like
// "agents.image.tag".
// Returns an error when there isn't a value associated for the key. It also
// returns an error if the provided key is "incomplete", meaning that there's a
// value associated for the key, but it is a map. For example, if the input map
// is:
//
//	map[string]interface{}{
//	  "agents": map[string]interface{}{
//	    "image": map[string]string{
//		     "tag": "7.39.0",
//	      "pullPolicy": "IfNotPresent",
//		   },
//		  },
//	}
//
// "agents.image.tag" and "agents.image.pullPolicy" are valid keys, but
// "agents.image" is not.
func getValue(m map[string]interface{}, dotSeparatedKey string) (string, error) {
	if dotSeparatedKey == "" {
		return "", fmt.Errorf("not found")
	}

	keys := strings.Split(dotSeparatedKey, ".")
	var obj interface{} = m

	for _, key := range keys {
		if obj == nil || reflect.TypeOf(obj).Kind() != reflect.Map {
			return "", fmt.Errorf("not found")
		}

		mapValue := reflect.ValueOf(obj)

		objValue := mapValue.MapIndex(reflect.ValueOf(key))

		if !objValue.IsValid() {
			return "", fmt.Errorf("not found")
		}

		obj = objValue.Interface()
	}

	if obj == nil || reflect.TypeOf(obj).Kind() == reflect.Map {
		return "", fmt.Errorf("not found")
	}

	return fmt.Sprintf("%v", reflect.ValueOf(obj)), nil
}
