// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fakeserver

import (
	"context"
	"encoding/base64"
	"strings"

	"google.golang.org/grpc/metadata"
)

func credentialsFromContext(ctx context.Context) (username, password string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ""
	}

	if values := md.Get("username"); len(values) > 0 {
		username = values[0]
	}
	if values := md.Get("password"); len(values) > 0 {
		password = values[0]
	}

	if username != "" && password != "" {
		return username, password
	}

	for _, value := range md.Get("authorization") {
		if !strings.HasPrefix(value, "Basic ") {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "Basic "))
		if err != nil {
			continue
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			continue
		}
		return parts[0], parts[1]
	}

	return username, password
}
