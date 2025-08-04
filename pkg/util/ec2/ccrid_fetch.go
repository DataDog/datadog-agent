// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const (
	imdsBaseURL = "http://169.254.169.254/latest/meta-data/"
)

var (
	getInstanceID = GetInstanceID
	getRegion     = GetRegion
	getAccountID  = GetAccountID
)

type fetcher struct {
	Name    string
	Attempt func(context.Context) (interface{}, error)

	mu    sync.Mutex
	cache interface{}
	err   error
	ready bool
}

func (f *fetcher) fetch(ctx context.Context) (interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ready {
		return f.cache, f.err
	}
	f.cache, f.err = f.Attempt(ctx)
	f.ready = true
	return f.cache, f.err
}

func (f *fetcher) fetchString(ctx context.Context) (string, error) {
	val, err := f.fetch(ctx)
	if err != nil {
		return "", err
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s returned non-string", f.Name)
	}
	return s, nil
}

var regionFetcher = fetcher{
	Name: "EC2 Region",
	Attempt: func(_ context.Context) (interface{}, error) {
		return httpGetMetadata("placement/region")
	},
}

// GetRegion returns the AWS region as reported by EC2 IMDS.
func GetRegion(ctx context.Context) (string, error) {
	return regionFetcher.fetchString(ctx)
}

func httpGetMetadata(path string) (string, error) {
	req, _ := http.NewRequest("GET", imdsBaseURL+path, nil)
	req.Header.Set("Metadata-Flavor", "Amazon")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata %q request failed: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata %q returned status %s", path, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// GetHostCCRID returns the EC2 instance ARN for use as host CCRID
func GetHostCCRID(ctx context.Context) (string, error) {
	instanceID, err := getInstanceID(ctx)
	if err != nil {
		return "", err
	}
	region, err := getRegion(ctx)
	if err != nil {
		return "", err
	}
	accountID, err := getAccountID(ctx)
	if err != nil {
		return "", err
	}

	arn := fmt.Sprintf("arn:aws:ec2:%s:%s:instance/%s", region, accountID, instanceID)
	return arn, nil
}
