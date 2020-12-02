// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util"
)

type failedTransactionRemovalPolicy struct {
	rootPath           string
	knownDomainFolders map[string]struct{}
	outdatedFileTime   time.Time
}

func newFailedTransactionRemovalPolicy(rootPath string, outdatedFileDayCount int) (*failedTransactionRemovalPolicy, error) {
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return nil, err
	}

	return &failedTransactionRemovalPolicy{
		rootPath:           rootPath,
		knownDomainFolders: make(map[string]struct{}),
		outdatedFileTime:   time.Now().Add(time.Duration(-outdatedFileDayCount*24) * time.Hour),
	}, nil
}

// RegisterDomain registers a domain name.
func (p *failedTransactionRemovalPolicy) RegisterDomain(domainName string) (string, error) {
	folder, err := p.getFolderPathForDomain(domainName)
	if err != nil {
		return "", err
	}

	p.knownDomainFolders[folder] = struct{}{}
	return folder, nil
}

// RemoveOutdatedFiles removes the outdated files.
// It removes files with extension retryTransactionsExtension:
// - For domains not registered
// - When a file is older than outDatedFileDayCount days
func (p *failedTransactionRemovalPolicy) RemoveOutdatedFiles() ([]string, error) {
	entries, err := ioutil.ReadDir(p.rootPath)
	if err != nil {
		return nil, err
	}

	var filesRemoved []string
	for _, domain := range entries {
		if domain.Mode().IsDir() {
			folderPath := path.Join(p.rootPath, domain.Name())
			var files []string
			if _, found := p.knownDomainFolders[folderPath]; found {
				files, err = p.removeOutdatedFiles(folderPath)
			} else {
				files, err = p.removeUnknownDomain(folderPath)
			}
			if err != nil {
				return nil, err
			}
			filesRemoved = append(filesRemoved, files...)
		}
	}
	return filesRemoved, nil
}

func (p *failedTransactionRemovalPolicy) getFolderPathForDomain(domainName string) (string, error) {
	// Use md5 for the folder name as the domainName is an url which can contain invalid charaters for a file path.
	h := md5.New()
	if _, err := io.WriteString(h, domainName); err != nil {
		return "", err
	}
	folder := fmt.Sprintf("%x", h.Sum(nil))

	return path.Join(p.rootPath, folder), nil
}

func (p *failedTransactionRemovalPolicy) removeUnknownDomain(folderPath string) ([]string, error) {
	files, err := p.removeRetryFiles(folderPath, func(filename string) bool { return true })

	// Try to remove the folder if it is empty
	_ = os.Remove(folderPath)
	return files, err
}

func (p *failedTransactionRemovalPolicy) removeOutdatedFiles(folderPath string) ([]string, error) {
	return p.removeRetryFiles(folderPath, func(filename string) bool {
		modTime, err := util.GetFileModTime(filename)
		if err != nil {
			return false
		}
		return modTime.Before(p.outdatedFileTime)
	})
}

func (p *failedTransactionRemovalPolicy) removeRetryFiles(folderPath string, predicate func(string) bool) ([]string, error) {
	files, err := p.getRetryFiles(folderPath)
	if err != nil {
		return nil, err
	}

	var filesRemoved []string
	for _, f := range files {
		if predicate(f) {
			if err = os.Remove(f); err != nil {
				return nil, err
			}
			filesRemoved = append(filesRemoved, f)
		}
	}
	return filesRemoved, nil
}

func (p *failedTransactionRemovalPolicy) getRetryFiles(folder string) ([]string, error) {
	entries, err := ioutil.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.Mode().IsRegular() && filepath.Ext(entry.Name()) == retryTransactionsExtension {
			files = append(files, path.Join(folder, entry.Name()))
		}
	}
	return files, nil
}
