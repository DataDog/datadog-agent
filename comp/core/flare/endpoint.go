// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type EndpointProvider struct {
	flareComp *Flare
}

func (EndpointProvider) Method() string {
	return "POST"
}

func (EndpointProvider) Route() string {
	return "/flare"
}

// ServeHTTP implements the http.Handler interface for the provided endpoint creating a flare
func (e EndpointProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var profile ProfileData

	if r.Body != http.NoBody {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
			return
		}

		if err := json.Unmarshal(body, &profile); err != nil {
			http.Error(w, log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
			return
		}
	}

	// Reset the `server_timeout` deadline for this connection as creating a flare can take some time
	conn := GetConnection(r)
	_ = conn.SetDeadline(time.Time{})

	var filePath string
	var err error
	log.Infof("Making a flare")
	filePath, err = e.flareComp.Create(profile, nil)

	if err != nil || filePath == "" {
		if err != nil {
			log.Errorf("The flare failed to be created: %s", err)
		} else {
			log.Warnf("The flare failed to be created")
		}
		http.Error(w, err.Error(), 500)
	}
	w.Write([]byte(filePath))
}

// GetConnection returns the connection for the request
func GetConnection(r *http.Request) net.Conn {
	return r.Context().Value(grpc.ConnContextKey).(net.Conn)
}
