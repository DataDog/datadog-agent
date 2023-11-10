// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ibm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// declare these as vars not const to ease testing
var (
	metadataURL      = "http://169.254.169.254"
	tokenEndpoint    = "/instance_identity/v1/token?version=2022-03-08"
	instanceEndpoint = "/metadata/v1/instance?version=2022-03-08"

	token *httputils.APIToken

	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "IBM"
)

func init() {
	token = httputils.NewAPIToken(getToken)
}

type tokenAnswer struct {
	Value     string `json:"access_token"`
	ExpiresAt string `json:"expires_at"`
}

type instanceAnswer struct {
	ID string `json:"id"`
}

func getToken(ctx context.Context) (string, time.Time, error) {
	res, err := httputils.Put(ctx,
		metadataURL+tokenEndpoint,
		map[string]string{
			"Metadata-Flavor": "ibm",
		},
		[]byte("{\"expires_in\": 3600}"),
		config.Datadog.GetDuration("ibm_metadata_timeout")*time.Second, config.Datadog)
	if err != nil {
		token.ExpirationDate = time.Now()
		return "", time.Time{}, err
	}

	data := tokenAnswer{}
	err = json.Unmarshal([]byte(res), &data)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("could not Unmarshal IBM token answer: %s", err)
	} else if data.Value == "" {
		return "", time.Time{}, fmt.Errorf("empty token returned by token API")
	}

	expiresAt, err := time.Parse(time.RFC3339, data.ExpiresAt)
	if err != nil {
		// if we can't parse the expire we expire the token right Now
		log.Debugf("could not parse token expire date: %s", err)
		return data.Value, time.Now(), nil
	}
	return data.Value, expiresAt, nil
}

// IsRunningOn returns true if the agent is running on IBM cloud
func IsRunningOn(ctx context.Context) bool {
	_, err := GetHostAliases(ctx)
	log.Debugf("running on IBM cloud: %v (%s)", err == nil, err)
	return err == nil
}

var instanceIDFetcher = cachedfetch.Fetcher{
	Name: "IBM instance name",
	Attempt: func(ctx context.Context) (interface{}, error) {
		if !config.IsCloudProviderEnabled(CloudProviderName) {
			return "", fmt.Errorf("IBM cloud provider is disabled by configuration")
		}

		t, err := token.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("IBM HostAliases: unable to get a token: %s", err)
		}

		res, err := httputils.Get(ctx,
			metadataURL+instanceEndpoint,
			map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", t),
			},
			config.Datadog.GetDuration("ibm_metadata_timeout")*time.Second, config.Datadog)
		if err != nil {
			return nil, fmt.Errorf("IBM HostAliases: unable to query metadata endpoint: %s", err)
		}

		if res == "" {
			return nil, fmt.Errorf("IBM '%s' returned empty id", metadataURL)
		}

		// We do not enforce config "metadata_endpoints_max_hostname_size" since the API returns all the
		// metadata for the host (around 2k payload).

		data := instanceAnswer{}
		err = json.Unmarshal([]byte(res), &data)
		if err != nil {
			return "", fmt.Errorf("could not Unmarshal IBM metadata answer: %s", err)
		}

		if data.ID == "" {
			return nil, fmt.Errorf("IBM cloud metdata endpoint returned an empty 'id'")
		}

		return []string{data.ID}, nil
	},
}

// GetHostAliases returns the VM name from the IBM Metadata api
func GetHostAliases(ctx context.Context) ([]string, error) {
	return instanceIDFetcher.FetchStringSlice(ctx)
}
