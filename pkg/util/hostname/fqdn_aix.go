// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"net"
	"strings"
	"time"
)

func getSystemFQDN() (string, error) {
	resolver := &net.Resolver{}
	return getSystemFQDNFromCNAME(resolver.LookupCNAME)
}

func getSystemFQDNFromCNAME(lookupCNAME func(context.Context, string) (string, error)) (string, error) {
	hostname, err := osHostname()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	fqdn, err := lookupCNAME(ctx, hostname)
	return strings.TrimSuffix(fqdn, "."), err
}
