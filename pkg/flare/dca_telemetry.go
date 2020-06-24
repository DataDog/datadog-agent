// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flare

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// QueryDCAMetrics gets the metrics payload exposed by the cluster agent
func QueryDCAMetrics() ([]byte, error) {
	r, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Datadog.GetInt("metrics_port")))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	return ioutil.ReadAll(r.Body)
}
