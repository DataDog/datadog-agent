// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import (
	"os"
	"reflect"

	"github.com/mohae/deepcopy"
)

// mergeProfiles merges two profiles config map
// we use deepcopy to lower risk of modifying original profiles
func mergeProfiles(profilesA ProfileConfigMap, profilesB ProfileConfigMap) ProfileConfigMap {
	profiles := make(ProfileConfigMap)
	for k, v := range profilesA {
		profiles[k] = deepcopy.Copy(v).(ProfileConfig)
	}
	for k, v := range profilesB {
		profiles[k] = deepcopy.Copy(v).(ProfileConfig)
	}
	return profiles
}

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// isStructEmpty returns true if the given struct is empty
func isStructEmpty(s interface{}) bool {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < v.NumField(); i++ {
		if !reflect.DeepEqual(v.Field(i).Interface(), reflect.Zero(v.Field(i).Type()).Interface()) {
			return false
		}
	}
	return true
}

// isEmpty returns true if the given field is empty
func isEmpty(field reflect.Value) bool {
	switch field.Kind() {
	case reflect.String:
		return field.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return field.Int() == 0
	case reflect.Bool:
		return !field.Bool()
	// Add other types as needed
	default:
		return false
	}
}
