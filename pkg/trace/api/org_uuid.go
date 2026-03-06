// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

const (
	orgDataPath     = "/api/v0.1/org"
	orgFetchMaxWait = 5 * time.Minute
)

// fetchOrgUUIDBackground fetches the org UUID from the remote config backend,
// computes base64url(Trunc60(SHA-256(uuid))), and stores it in r.orgUUIDOPM.
// It retries with exponential backoff until successful or done is closed.
func (r *HTTPReceiver) fetchOrgUUIDBackground(done <-chan struct{}) {
	if len(r.conf.Endpoints) == 0 || r.conf.Endpoints[0].APIKey == "" || r.conf.Site == "" {
		return
	}

	apiKey := r.conf.Endpoints[0].APIKey
	url := "https://config." + r.conf.Site + orgDataPath
	client := r.conf.NewHTTPClient()

	wait := time.Duration(0)
	for {
		if wait > 0 {
			select {
			case <-done:
				return
			case <-time.After(wait):
			}
		}

		uuid, err := doFetchOrgUUID(client, url, apiKey)
		if err != nil {
			if wait == 0 {
				wait = 5 * time.Second
			} else {
				wait = min(wait*2, orgFetchMaxWait)
			}
			log.Debugf("Failed to fetch org UUID for /info endpoint: %v, retrying in %v", err, wait)
			continue
		}

		// opm = base64url( Trunc60( SHA-256(org_uuid) ) )
		// SHA-256 → 32 bytes. base64url with RawURLEncoding of the first 8 bytes
		// gives 11 chars; the first 10 chars encode exactly 60 bits (10 × 6 bits).
		h := sha256.Sum256([]byte(uuid))
		opm := base64.RawURLEncoding.EncodeToString(h[:8])[:10]
		r.orgUUIDOPM.Store(opm)
		log.Debugf("Org UUID hash stored for /info endpoint")
		return
	}
}

type doer interface {
	Do(*http.Request) (*http.Response, error)
}

func doFetchOrgUUID(client doer, url, apiKey string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("DD-Api-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to issue request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var orgData pbgo.OrgDataResponse
	if err := proto.Unmarshal(body, &orgData); err != nil {
		return "", fmt.Errorf("failed to unmarshal org data response: %w", err)
	}

	if orgData.GetUuid() == "" {
		return "", fmt.Errorf("org UUID is empty")
	}
	return orgData.GetUuid(), nil
}
