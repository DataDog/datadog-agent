// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
)

// HostnameProvider parses a file from the 'filename' option and returns the content
// as the hostname
func HostnameProvider(ctx context.Context, options map[string]interface{}) (string, error) {
	if options == nil {
		return "", fmt.Errorf("'file' hostname provider requires a 'filename' field in options")
	}

	filenameVal, ok := options["filename"]
	if !ok {
		return "", fmt.Errorf("'file' hostname provider requires a 'filename' field in options")
	}

	filename := fmt.Sprintf("%s", filenameVal)
	fileContent, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("Could not read hostname from %s: %v", filename, err)
	}

	hostname := strings.TrimSpace(string(fileContent))

	err = validate.ValidHostname(hostname)
	if err != nil {
		return "", err
	}

	return hostname, nil
}
