// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"bytes"
	"encoding/json"
)

// PrettyPrintJSON pretty prints a json
func PrettyPrintJSON(data interface{}, tab string) ([]byte, error) {
	unformatted, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := json.Indent(&buffer, unformatted, "", "\t"); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
