// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package uptane

import (
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/DataDog/go-tuf/client"
)

// cdnRemoteStore implements go-tuf's RemoteStore
// It is an HTTP interface to an authenticated remote server that serves an uptane repository
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
type cdnRemoteStore struct {
	httpClient     RequestDoer
	host           string
	site           string
	apiKey         string
	repositoryType string

	authnToken string
}

// RequestDoer is an interface that abstracts the http.Client.Do method
type RequestDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type cdnRemoteConfigStore struct {
	cdnRemoteStore
}

type cdnRemoteDirectorStore struct {
	cdnRemoteStore
}

func newCDNRemoteConfigStore(client *http.Client, host, site, apiKey string) *cdnRemoteConfigStore {
	return &cdnRemoteConfigStore{
		cdnRemoteStore: cdnRemoteStore{
			httpClient:     client,
			host:           host,
			site:           site,
			apiKey:         apiKey,
			repositoryType: "config",
		},
	}
}

func newCDNRemoteDirectorStore(client *http.Client, host, site, apiKey string) *cdnRemoteDirectorStore {
	return &cdnRemoteDirectorStore{
		cdnRemoteStore: cdnRemoteStore{
			httpClient:     client,
			host:           host,
			site:           site,
			apiKey:         apiKey,
			repositoryType: "director",
		},
	}
}

func (s *cdnRemoteStore) newAuthenticatedHTTPReq(method, p string) (*http.Request, error) {
	req, err := http.NewRequest(method, s.host, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("X-Dd-Api-Key", s.apiKey)
	if s.authnToken != "" {
		req.Header.Add("Authorization", s.authnToken)
	}

	req.URL.Scheme = "https"
	req.URL.Host = s.host
	req.URL.Path = "/" + path.Join(s.site, p)
	req.Host = s.host

	return req, err
}

func (s *cdnRemoteStore) updateAuthnToken(resp *http.Response) {
	authToken := resp.Header.Get("X-Dd-Refreshed-Authorization")
	if authToken != "" {
		s.authnToken = authToken
	}
}

func (s *cdnRemoteStore) getRCFile(path string) (io.ReadCloser, int64, error) {
	req, err := s.newAuthenticatedHTTPReq("GET", path)
	if err != nil {
		return nil, 0, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, 0, client.ErrNotFound{File: path}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	s.updateAuthnToken(resp)
	return resp.Body, resp.ContentLength, nil
}

// GetMeta implements go-tuf's RemoteStore.GetMeta
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
func (s *cdnRemoteStore) GetMeta(p string) (io.ReadCloser, int64, error) {
	return s.getRCFile(path.Join(s.repositoryType, p))
}

// GetTarget implements go-tuf's RemoteStore.GetTarget
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
func (s *cdnRemoteStore) GetTarget(path string) (io.ReadCloser, int64, error) {
	return s.getRCFile(path)
}
