// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/utils"
)

// EndpointProvider wraps the flare component with a http.Handler interface
type EndpointProvider struct {
	flareComp *flare
}

// ServeHTTP implements the http.Handler interface for the provided endpoint creating a flare
func (e EndpointProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var profile ProfileData

	if r.Body != http.NoBody {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, e.flareComp.log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
			return
		}

		if err := json.Unmarshal(body, &profile); err != nil {
			http.Error(w, e.flareComp.log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
			return
		}
	}

	// Reset the `server_timeout` deadline for this connection as creating a flare can take some time
	conn := utils.GetConnection(r)
	_ = conn.SetDeadline(time.Time{})

	var filePath string
	var err error
	e.flareComp.log.Infof("Making a flare")
	filePath, err = e.flareComp.Create(profile, nil)

	if err != nil || filePath == "" {
		if err != nil {
			e.flareComp.log.Errorf("The flare failed to be created: %s", err)
		} else {
			e.flareComp.log.Warnf("The flare failed to be created")
		}
		http.Error(w, err.Error(), 500)
	}
	w.Write([]byte(filePath))
}
