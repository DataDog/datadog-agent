// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/DataDog/go-tuf/client"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type role string

const (
	roleRoot      role = "root"
	roleTargets   role = "targets"
	roleSnapshot  role = "snapshot"
	roleTimestamp role = "timestamp"
)

// remoteStore implements go-tuf's RemoteStore
// Its goal is to serve TUF metadata updates coming to the backend in a way go-tuf understands
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
type remoteStore struct {
	targetStore *targetStore
	metas       map[role]map[uint64][]byte
}

func newRemoteStore(targetStore *targetStore) remoteStore {
	return remoteStore{
		metas: map[role]map[uint64][]byte{
			roleRoot:      make(map[uint64][]byte),
			roleTargets:   make(map[uint64][]byte),
			roleSnapshot:  make(map[uint64][]byte),
			roleTimestamp: make(map[uint64][]byte),
		},
		targetStore: targetStore,
	}
}

func (s *remoteStore) resetRole(r role) {
	s.metas[r] = make(map[uint64][]byte)
}

func (s *remoteStore) latestVersion(r role) uint64 {
	latestVersion := uint64(0)
	for v := range s.metas[r] {
		if v > latestVersion {
			latestVersion = v
		}
	}
	return latestVersion
}

// GetMeta implements go-tuf's RemoteStore.GetMeta
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
func (s *remoteStore) GetMeta(path string) (io.ReadCloser, int64, error) {
	metaPath, err := parseMetaPath(path)
	if err != nil {
		return nil, 0, err
	}
	roleVersions, roleFound := s.metas[metaPath.role]
	if !roleFound {
		return nil, 0, client.ErrNotFound{File: path}
	}
	version := metaPath.version
	if !metaPath.versionSet {
		if metaPath.role != roleTimestamp {
			return nil, 0, client.ErrNotFound{File: path}
		}
		version = s.latestVersion(metaPath.role)
	}
	requestedVersion, versionFound := roleVersions[version]
	if !versionFound {
		return nil, 0, client.ErrNotFound{File: path}
	}
	return io.NopCloser(bytes.NewReader(requestedVersion)), int64(len(requestedVersion)), nil
}

// GetTarget implements go-tuf's RemoteStore.GetTarget
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
func (s *remoteStore) GetTarget(targetPath string) (stream io.ReadCloser, size int64, err error) {
	target, found, err := s.targetStore.getTargetFile(targetPath)
	if err != nil {
		return nil, 0, err
	}
	if !found {
		return nil, 0, client.ErrNotFound{File: targetPath}
	}
	return io.NopCloser(bytes.NewReader(target)), int64(len(target)), nil
}

type remoteStoreDirector struct {
	remoteStore
}

func newRemoteStoreDirector(targetStore *targetStore) *remoteStoreDirector {
	return &remoteStoreDirector{remoteStore: newRemoteStore(targetStore)}
}

func (sd *remoteStoreDirector) update(update *pbgo.LatestConfigsResponse) {
	if update == nil {
		return
	}
	if update.DirectorMetas == nil {
		return
	}
	metas := update.DirectorMetas
	for _, root := range metas.Roots {
		sd.metas[roleRoot][root.Version] = root.Raw
	}
	if metas.Timestamp != nil {
		sd.resetRole(roleTimestamp)
		sd.metas[roleTimestamp][metas.Timestamp.Version] = metas.Timestamp.Raw
	}
	if metas.Snapshot != nil {
		sd.resetRole(roleSnapshot)
		sd.metas[roleSnapshot][metas.Snapshot.Version] = metas.Snapshot.Raw
	}
	if metas.Targets != nil {
		sd.resetRole(roleTargets)
		sd.metas[roleTargets][metas.Targets.Version] = metas.Targets.Raw
	}
}

type remoteStoreConfig struct {
	remoteStore
}

func newRemoteStoreConfig(targetStore *targetStore) *remoteStoreConfig {
	return &remoteStoreConfig{
		remoteStore: newRemoteStore(targetStore),
	}
}

func (sc *remoteStoreConfig) update(update *pbgo.LatestConfigsResponse) {
	if update == nil {
		return
	}
	if update.ConfigMetas == nil {
		return
	}
	metas := update.ConfigMetas
	for _, root := range metas.Roots {
		sc.metas[roleRoot][root.Version] = root.Raw
	}
	for _, delegatedMeta := range metas.DelegatedTargets {
		role := role(delegatedMeta.Role)
		sc.resetRole(role)
		sc.metas[role][delegatedMeta.Version] = delegatedMeta.Raw
	}
	if metas.Timestamp != nil {
		sc.resetRole(roleTimestamp)
		sc.metas[roleTimestamp][metas.Timestamp.Version] = metas.Timestamp.Raw
	}
	if metas.Snapshot != nil {
		sc.resetRole(roleSnapshot)
		sc.metas[roleSnapshot][metas.Snapshot.Version] = metas.Snapshot.Raw
	}
	if metas.TopTargets != nil {
		sc.resetRole(roleTargets)
		sc.metas[roleTargets][metas.TopTargets.Version] = metas.TopTargets.Raw
	}
}

// cdnRemoteStore implements go-tuf's RemoteStore
// It is an HTTP interface to an authenticated remote server that serves an uptane repository
// See https://pkg.go.dev/github.com/DataDog/go-tuf/client#RemoteStore
type cdnRemoteStore struct {
	httpClient     RequestDoer
	host           string
	pathPrefix     string
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

// getCDNHostnameFromSite returns the staging or production CDN hostname for a given site.
// Site can be any of the (non-fed) documented DD sites per https://docs.datadoghq.com/getting_started/site/
func getCDNHostnameFromSite(site string) string {
	s := strings.TrimPrefix(site, "https://")
	switch s {
	// staging:
	case "datad0g.com":
		return "remote-config.datad0g.com"
	// prod:
	case "ap1.datadoghq.com":
		return "remote-config.datadoghq.com"
	case "us5.datadoghq.com":
		return "remote-config.datadoghq.com"
	case "us3.datadoghq.com":
		return "remote-config.datadoghq.com"
	case "app.datadoghq.eu":
		return "remote-config.datadoghq.com"
	case "app.datadoghq.com":
		return "remote-config.datadoghq.com"
	}
	return "remote-config.datadoghq.com"
}

// Trims any schemas or non-datacenter related subdomains from the site to get the path prefix for the CDN
// e.g. https://us3.datadoghq.com -> us3.datadoghq.com
// e.g. https://app.datadoghq.com -> datadoghq.com
func getCDNPathPrefixFromSite(site string) string {
	s := strings.TrimPrefix(site, "https://app.")
	s = strings.TrimPrefix(s, "https://")
	return s
}

func newCDNRemoteConfigStore(client *http.Client, site, apiKey string) *cdnRemoteConfigStore {
	return &cdnRemoteConfigStore{
		cdnRemoteStore: cdnRemoteStore{
			httpClient:     client,
			host:           getCDNHostnameFromSite(site),
			pathPrefix:     getCDNPathPrefixFromSite(site),
			apiKey:         apiKey,
			repositoryType: "config",
		},
	}
}

func newCDNRemoteDirectorStore(client *http.Client, site, apiKey string) *cdnRemoteDirectorStore {
	return &cdnRemoteDirectorStore{
		cdnRemoteStore: cdnRemoteStore{
			httpClient:     client,
			host:           getCDNHostnameFromSite(site),
			pathPrefix:     getCDNPathPrefixFromSite(site),
			apiKey:         apiKey,
			repositoryType: "director",
		},
	}
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
	req.URL.Path = "/" + path.Join(s.pathPrefix, p)
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
