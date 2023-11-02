// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package expvarcollector allows external pacjages to register the expvar that wants to report on for agent status command.
package expvarcollector

var expvarRegistry = map[string]func() (interface{}, error){}

// RegisterExpvarReport allow components to register the function to collect
// expvar variables.
func RegisterExpvarReport(key string, report func() (interface{}, error)) {
	expvarRegistry[key] = report
}

// Report iterates over the registered collect function and append the result into the stats map
func Report(stats map[string]interface{}) (map[string]interface{}, []error) {
	errors := []error{}
	for key, report := range expvarRegistry {
		result, err := report()
		if err != nil {
			errors = append(errors, err)
		} else {
			stats[key] = result
		}
	}
	return stats, errors
}
