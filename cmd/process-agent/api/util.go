// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/json"
	"net/http"
)

func writeError(err error, code int, w http.ResponseWriter) {
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	http.Error(w, string(body), code)
}
