// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package repo is utilities for a DNFv2 repository
package repo

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/sassoftware/go-rpmutils"
	"gopkg.in/ini.v1"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/internal/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/types"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/xmlite"
)

// Repo is a DNFv2 repository
type Repo struct {
	SectionName   string
	Name          string
	BaseURL       string
	MirrorList    string
	MetaLink      string
	Type          string
	Enabled       bool
	GpgCheck      bool
	GpgKeys       []string
	SSLVerify     bool
	SSLClientKey  string
	SSLClientCert string
	SSLCaCert     string
}

// ReadFromDir parses repo files from the provided directory
func ReadFromDir(repoDir string) ([]Repo, error) {
	repoFiles, err := filepath.Glob(utils.HostEtcJoin(repoDir, "*.repo"))
	if err != nil {
		return nil, err
	}

	repos := make([]Repo, 0)
	for _, repoFile := range repoFiles {
		cfg, err := ini.Load(repoFile)
		if err != nil {
			return nil, err
		}

		for _, section := range cfg.Sections() {
			if section.Name() == "DEFAULT" {
				continue
			}

			repo := Repo{}
			repo.SectionName = section.Name()
			repo.Name = section.Key("name").String()
			repo.BaseURL = section.Key("baseurl").String()
			repo.MirrorList = section.Key("mirrorlist").String()
			repo.MetaLink = section.Key("metalink").String()
			repo.Type = section.Key("type").String()
			repo.Enabled = section.Key("enabled").MustBool()
			repo.GpgCheck = section.Key("gpgcheck").MustBool()
			repo.GpgKeys = strings.Split(section.Key("gpgkey").String(), ",")
			repo.SSLVerify = section.Key("sslverify").MustBool(true)
			repo.SSLClientKey = section.Key("sslclientkey").String()
			repo.SSLClientCert = section.Key("sslclientcert").String()
			repo.SSLCaCert = section.Key("sslcacert").String()

			// hack for yast2 repo support
			if repo.Type == "yast2" && repo.BaseURL != "" {
				repo.BaseURL += "suse/"
			}

			repos = append(repos, repo)
		}
	}
	return repos, nil
}

// PkgInfo is DNFv2 package information
type PkgInfo struct {
	Header   PkgInfoHeader
	Location string
	Checksum *types.Checksum
}

// PkgInfoHeader is a DNFv2 package header
type PkgInfoHeader struct {
	Name string
	types.Version
	Arch string
}

// PkgMatchFunc is function that returns true if the provided package info matches
type PkgMatchFunc = func(*PkgInfoHeader) bool

func (r *Repo) createHTTPClient() (*utils.HTTPClient, error) {
	var certs []tls.Certificate
	if r.SSLClientCert != "" || r.SSLClientKey != "" {
		cert, err := tls.LoadX509KeyPair(utils.HostEtcJoin(r.SSLClientCert), utils.HostEtcJoin(r.SSLClientKey))
		if err != nil {
			return nil, fmt.Errorf("load SSL certificate: %w", err)
		}
		certs = append(certs, cert)
	}

	var certPool *x509.CertPool
	if r.SSLCaCert != "" {
		certPool = x509.NewCertPool()
		customPem, err := os.ReadFile(utils.HostEtcJoin(r.SSLCaCert))
		if err != nil {
			return nil, fmt.Errorf("read custom CA cert: %s", err)
		}
		if !certPool.AppendCertsFromPEM(customPem) {
			return nil, errors.New("no custom CA certs added to cert pool")
		}
	}

	inner := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !r.SSLVerify,
				Certificates:       certs,
				RootCAs:            certPool,
			},
		},
	}

	return utils.NewHTTPClientFromInner(inner), nil
}

// FetchPackage fetches the provided package and returns the package information and RPM data.
func (r *Repo) FetchPackage(ctx context.Context, pkgMatcher PkgMatchFunc) (*PkgInfo, []byte, error) {
	httpClient, err := r.createHTTPClient()
	if err != nil {
		return nil, nil, err
	}

	repoMd, err := r.fetchRepoMD(ctx, httpClient)
	if err != nil {
		return nil, nil, err
	}

	fetchURL, err := r.fetchURL(ctx, httpClient)
	if err != nil {
		return nil, nil, err
	}

	var entityList openpgp.EntityList
	if r.GpgCheck {
		el, err := readGPGKeys(ctx, httpClient, r.GpgKeys)
		// if we found keys we can ignore the error
		if err != nil && len(el) == 0 {
			return nil, nil, fmt.Errorf("read gpg key: %w", err)
		}
		entityList = el
	}

	pkgInfo, err := r.fetchPackageFromList(ctx, httpClient, repoMd, pkgMatcher)
	if err != nil {
		return nil, nil, fmt.Errorf("find valid package from repo %s: %w", r.Name, err)
	}

	pkgURL, err := utils.URLJoinPath(fetchURL, pkgInfo.Location)
	if err != nil {
		return nil, nil, err
	}

	pkgRpmData, err := httpClient.GetWithChecksum(ctx, pkgURL, pkgInfo.Checksum)
	if err != nil {
		return nil, nil, err
	}

	if r.GpgCheck {
		_, _, err = rpmutils.Verify(bytes.NewReader(pkgRpmData), entityList)
		if err != nil {
			return nil, nil, err
		}
	}

	return pkgInfo, pkgRpmData, err
}

func readGPGKeys(ctx context.Context, httpClient *utils.HTTPClient, gpgKeys []string) (openpgp.EntityList, error) {
	visited := make(map[string]bool, len(gpgKeys))

	var entities openpgp.EntityList
	var allerrors error

	for _, gpgKey := range gpgKeys {
		if visited[gpgKey] {
			// this key is already loaded
			continue
		}
		visited[gpgKey] = true

		newEntities, err := readGPGKey(ctx, httpClient, gpgKey)
		if err != nil {
			allerrors = errors.Join(allerrors, err)
			continue
		}
		entities = append(entities, newEntities...)
	}
	return entities, allerrors
}

func readGPGKey(ctx context.Context, httpClient *utils.HTTPClient, gpgKey string) (openpgp.EntityList, error) {
	gpgKeyURL, err := url.Parse(gpgKey)
	if err != nil {
		return nil, err
	}

	var publicKeyReader io.Reader
	if gpgKeyURL.Scheme == "file" {
		publicKeyFile, err := os.Open(utils.HostEtcJoin(gpgKeyURL.Path))
		if err != nil {
			return nil, err
		}
		defer publicKeyFile.Close()
		publicKeyReader = publicKeyFile
	} else if gpgKeyURL.Scheme == "http" || gpgKeyURL.Scheme == "https" {
		content, err := httpClient.Get(ctx, gpgKey)
		if err != nil {
			return nil, err
		}
		publicKeyReader = bytes.NewReader(content)
	} else {
		return nil, fmt.Errorf("only file and http(s) scheme are supported for gpg key: %s", gpgKey)
	}

	newEntities, err := openpgp.ReadArmoredKeyRing(publicKeyReader)
	if err != nil {
		return nil, err
	}
	return newEntities, nil
}

const repomdSubpath = "repodata/repomd.xml"

// fetchRepoMD fetches the repomd XML content
func (r *Repo) fetchRepoMD(ctx context.Context, httpClient *utils.HTTPClient) (*types.Repomd, error) {
	fetchURL, err := r.fetchURL(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	repoMDUrl := fetchURL
	if !utils.URLHasSuffix(repoMDUrl, "repomd.xml") {
		withFile, err := utils.URLJoinPath(fetchURL, repomdSubpath)
		if err != nil {
			return nil, err
		}
		repoMDUrl = withFile
	}

	repoMd, err := utils.GetAndUnmarshalXML[types.Repomd](ctx, httpClient, repoMDUrl, nil)
	if err != nil {
		return nil, err
	}

	return repoMd, nil
}

// fetchURL returns the url used for fetching
func (r *Repo) fetchURL(ctx context.Context, httpClient *utils.HTTPClient) (string, error) {
	if r.BaseURL != "" {
		return r.BaseURL, nil
	}

	if r.MirrorList != "" {
		baseURL, err := fetchURLFromMirrorList(ctx, httpClient, r.MirrorList)
		if err != nil {
			return "", err
		}
		r.BaseURL = baseURL
		return r.BaseURL, nil
	}

	if r.MetaLink != "" {
		baseurl, err := fetchURLFromMetaLink(ctx, httpClient, r.MetaLink)
		if err != nil {
			return "", err
		}
		r.BaseURL = baseurl
		return r.BaseURL, nil
	}

	return "", fmt.Errorf("unable to get a base URL for this repo `%s`", r.Name)
}

func fetchURLFromMirrorList(ctx context.Context, httpClient *utils.HTTPClient, mirrorListURL string) (string, error) {
	mirrorListData, err := httpClient.Get(ctx, mirrorListURL)
	if err != nil {
		return "", err
	}

	mirrors := make([]string, 0)
	sc := bufio.NewScanner(bytes.NewReader(mirrorListData))
	for sc.Scan() {
		if sc.Err() != nil {
			return "", err
		}

		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}

		mirrors = append(mirrors, sc.Text())
	}

	if len(mirrors) == 0 {
		return "", errors.New("no mirror available")
	}
	return mirrors[0], nil
}

func fetchURLFromMetaLink(ctx context.Context, httpClient *utils.HTTPClient, metaLinkURL string) (string, error) {
	metalink, err := utils.GetAndUnmarshalXML[types.MetaLink](ctx, httpClient, metaLinkURL, nil)
	if err != nil {
		return "", err
	}

	for _, file := range metalink.Files.Files {
		if file.Name == "repomd.xml" {
			urls := make([]types.MetaLinkFileResourceURL, 0, len(file.Resources.Urls))
			for _, resURL := range file.Resources.Urls {
				if resURL.Protocol == "http" || resURL.Protocol == "https" {
					urls = append(urls, resURL)
				}
			}

			if len(urls) == 0 {
				return "", errors.New("no url for `repomd.xml` resource")
			}

			sort.Slice(urls, func(i, j int) bool {
				return urls[j].Preference < urls[i].Preference
			})

			repomdURL := strings.TrimSuffix(urls[0].URL, repomdSubpath)
			return repomdURL, nil
		}
	}

	return "", fmt.Errorf("fetch base URL from meta link: %s", metaLinkURL)
}

func (r *Repo) fetchPackageFromList(ctx context.Context, httpClient *utils.HTTPClient, repoMd *types.Repomd, pkgMatcher PkgMatchFunc) (*PkgInfo, error) {
	fetchURL, err := r.fetchURL(ctx, httpClient)
	if err != nil {
		return nil, err
	}

	for _, d := range repoMd.Data {
		if d.Type == "primary" {
			primaryURL, err := utils.URLJoinPath(fetchURL, d.Location.Href)
			if err != nil {
				return nil, err
			}

			primaryContent, err := httpClient.GetWithChecksum(ctx, primaryURL, &d.OpenChecksum)
			if err != nil {
				return nil, err
			}

			var pkgInfo *PkgInfo
			for _, path := range []xmlPkgPath{fastPath, slowPath} {
				pkgInfo, err = func(path xmlPkgPath) (*PkgInfo, error) {
					return path(bytes.NewReader(primaryContent), pkgMatcher)
				}(path)

				if err != nil {
					continue
				}
				if pkgInfo != nil {
					return pkgInfo, nil
				}

				// if we found nothing but no error we don't run the slow path
				break
			}

			// if the slow path returns an error we fire it
			if err != nil {
				return nil, err
			}
		}
	}

	return nil, errors.New("no matching package found")
}

type xmlPkgPath = func(io.Reader, PkgMatchFunc) (*PkgInfo, error)

func fastPath(reader io.Reader, pkgMatcher PkgMatchFunc) (*PkgInfo, error) {
	handler := &pkgHandler{
		matcher: pkgMatcher,
	}
	decoder := xmlite.NewLiteDecoder(reader, handler)
	if err := decoder.Parse(); err != nil {
		return nil, err
	}

	return handler.winner, nil
}

type parseState int

const (
	start parseState = iota
	inPackage
	inArch
	inLocation
	inFormat
	inProvides
	inEntry
	inChecksum
)

type pkgHandler struct {
	err     error
	matcher PkgMatchFunc
	winner  *PkgInfo
	state   parseState
	current *tempPkgInfo
}

type tempPkgInfo struct {
	arch      string
	location  string
	checksum  *types.Checksum
	currEntry *tempProvides
}

type tempProvides struct {
	name  string
	epoch string
	ver   string
	rel   string
}

// StartTag is the XMLLite decoder handler for start XML tags
func (ph *pkgHandler) StartTag(name []byte) {
	switch string(name) {
	case "package":
		ph.state = inPackage
		ph.current = &tempPkgInfo{}
	case "arch":
		if ph.state == inPackage {
			ph.state = inArch
		}
	case "location":
		if ph.state == inPackage {
			ph.state = inLocation
		}
	case "checksum":
		if ph.state == inPackage {
			ph.state = inChecksum
			if ph.current != nil {
				ph.current.checksum = &types.Checksum{}
			}
		}
	case "format":
		if ph.state == inPackage {
			ph.state = inFormat
		}
	case "rpm:provides":
		if ph.state == inFormat {
			ph.state = inProvides
		}
	case "rpm:entry":
		if ph.state == inProvides {
			ph.state = inEntry
			if ph.current != nil {
				ph.current.currEntry = &tempProvides{}
			}
		}
	}
}

// EndTag is the XMLLite decoder handler for end XML tags
func (ph *pkgHandler) EndTag(name []byte) {
	switch string(name) {
	case "package":
		ph.state = start
		ph.current = nil
	case "arch":
		if ph.state == inArch {
			ph.state = inPackage
		}
	case "location":
		if ph.state == inLocation {
			ph.state = inPackage
		}
	case "checksum":
		if ph.state == inChecksum {
			ph.state = inPackage
		}
	case "format":
		if ph.state == inFormat {
			ph.state = inPackage
		}
	case "rpm:provides":
		if ph.state == inProvides {
			ph.state = inFormat
		}
	case "rpm:entry":
		if ph.state == inEntry {
			ph.state = inProvides
			if ph.current != nil && ph.current.currEntry != nil && !strings.Contains(ph.current.currEntry.name, "(") && ph.matcher != nil && ph.winner == nil {
				if ph.current.arch == "" {
					ph.err = errors.New("arch declared after entry, fast path impossible")
				}

				pkgInfo := &PkgInfo{
					Header: PkgInfoHeader{
						Name: ph.current.currEntry.name,
						Version: types.Version{
							Epoch: ph.current.currEntry.epoch,
							Ver:   ph.current.currEntry.ver,
							Rel:   ph.current.currEntry.rel,
						},
						Arch: ph.current.arch,
					},
					Location: ph.current.location,
					Checksum: ph.current.checksum,
				}

				if ph.matcher(&pkgInfo.Header) {
					ph.winner = pkgInfo
				}

				ph.current.currEntry = nil
			}
		}
	}
}

// Attr is the XMLLite decoder handler for attribute data
func (ph *pkgHandler) Attr(name, value []byte) {
	if ph.current == nil {
		return
	}

	if ph.state == inLocation && string(name) == "href" {
		ph.current.location = string(value)
	} else if ph.state == inEntry && ph.current.currEntry != nil {
		switch string(name) {
		case "name":
			ph.current.currEntry.name = string(value)
		case "epoch":
			ph.current.currEntry.epoch = string(value)
		case "ver":
			ph.current.currEntry.ver = string(value)
		case "rel":
			ph.current.currEntry.rel = string(value)
		}
	} else if ph.state == inChecksum && string(name) == "type" {
		if ph.current.checksum != nil {
			ph.current.checksum.Type = string(value)
		}
	}
}

// CharData is the XMLLite decoder handler for character data
func (ph *pkgHandler) CharData(value []byte) {
	if ph.current == nil {
		return
	}

	switch ph.state {
	case inArch:
		ph.current.arch = string(value)
	case inChecksum:
		if ph.current.checksum != nil {
			ph.current.checksum.Hash = string(value)
		}
	default:
		return
	}
}

func slowPath(reader io.Reader, pkgMatcher PkgMatchFunc) (*PkgInfo, error) {
	d := xml.NewDecoder(reader)
	for {
		tok, err := d.Token()
		if tok == nil || err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		switch ty := tok.(type) {
		case xml.StartElement:
			if ty.Name.Local == "package" {
				var pkg types.Package
				if err = d.DecodeElement(&pkg, &ty); err != nil {
					return nil, err
				}

				for _, provides := range pkg.Provides {

					pkgInfo := &PkgInfo{
						Header: PkgInfoHeader{
							Name:    provides.Name,
							Version: provides.Version,
							Arch:    pkg.Arch,
						},
						Location: pkg.Location.Href,
						Checksum: &pkg.Checksum,
					}

					if pkgMatcher(&pkgInfo.Header) {
						return pkgInfo, nil
					}
				}
			}
		default:
		}
	}

	return nil, nil
}
