// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"io/ioutil"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultURL = "http://metadata.google.internal/computeMetadata/v1/instance/id"
const defaultTimeout = 300 * time.Millisecond

type Config struct {
	url     string
	timeout time.Duration
}

func GetDefaultConfig() *Config {
	return &Config{
		url:     defaultURL,
		timeout: defaultTimeout,
	}
}

func GetContainerID(config *Config) string {
	client := &http.Client{
		Timeout: config.timeout,
	}
	req, err := http.NewRequest(http.MethodGet, config.url, nil)
	if err != nil {
		log.Error("unable to build the metadata request, defaulting to unknown-id")
		return "unknown-id"
	}
	req.Header.Add("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil {
		log.Error("unable to get the instance id, defaulting to unknown-id")
		return "unknown-id"
	}
	data, _ := ioutil.ReadAll(res.Body)
	return string(data)
}
