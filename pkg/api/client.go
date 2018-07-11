// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package api

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// RetrieveJSON allows to quickly query an api endpoint as JSON
func RetrieveJSON(path string, target interface{}) error {
	c := util.GetClient(false) // FIX: get certificates right then make this true
	url := fmt.Sprintf("https://localhost:%v/%s", config.Datadog.GetInt("cmd_port"), path)

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	r, err := util.DoGet(c, url)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			err = fmt.Errorf(e)
		}
		return err
	}

	if err = json.Unmarshal(r, target); err != nil {
		return fmt.Errorf("Error unmarshalling json: %s", err)
	}
	return nil
}
