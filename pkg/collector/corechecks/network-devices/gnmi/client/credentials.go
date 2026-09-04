// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"context"
	"encoding/base64"

	"google.golang.org/grpc/credentials"
)

// passCred implements gNMI username/password authentication via gRPC metadata.
// It mirrors the OpenConfig gNMI reference client and also sends HTTP Basic auth
// for targets that expect an Authorization header.
type passCred struct {
	username string
	password string
	secure   bool
}

func newPassCred(username, password string, secure bool) credentials.PerRPCCredentials {
	return &passCred{
		username: username,
		password: password,
		secure:   secure,
	}
}

func (pc *passCred) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	metadata := map[string]string{
		"username": pc.username,
		"password": pc.password,
	}
	if pc.username != "" && pc.password != "" {
		auth := pc.username + ":" + pc.password
		metadata["authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	}
	return metadata, nil
}

func (pc *passCred) RequireTransportSecurity() bool {
	return pc.secure
}
