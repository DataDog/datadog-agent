// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// FileRemovalPolicy handles the removal policy for `.retry` files.
type FileRemovalPolicy struct {
	rootPath           string
	knownDomainFolders map[string]struct{}
	outdatedFileTime   time.Time
	telemetry          FileRemovalPolicyTelemetry
}

// NewFileRemovalPolicy creates a new instance of FileRemovalPolicy
func NewFileRemovalPolicy(
	rootPath string,
	outdatedFileDayCount int,
	telemetry FileRemovalPolicyTelemetry) (*FileRemovalPolicy, error) {
	if err := os.MkdirAll(rootPath, 0700); err != nil {
		return nil, err
	}

	permission, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}
	if err := permission.RestrictAccessToUser(rootPath); err != nil {
		return nil, err
	}

	telemetry.setNewRemovalPolicyCount(1)

	return &FileRemovalPolicy{
		rootPath:           rootPath,
		knownDomainFolders: make(map[string]struct{}),
		outdatedFileTime:   time.Now().Add(time.Duration(-outdatedFileDayCount*24) * time.Hour),
		telemetry:          telemetry,
	}, nil
}

// RegisterDomain registers a domain name.
func (p *FileRemovalPolicy) RegisterDomain(domainName string) (string, error) {
	folder, err := p.getFolderPathForDomain(domainName)
	if err != nil {
		return "", err
	}

	p.knownDomainFolders[folder] = struct{}{}
	p.telemetry.setRegisteredDomainCount(len(p.knownDomainFolders), domainName)
	return folder, nil
}

// RemoveOutdatedFiles removes the outdated files when a file is
// older than outDatedFileDayCount days.
func (p *FileRemovalPolicy) RemoveOutdatedFiles() ([]string, error) {
	outdatedFiles, err := p.forEachDomainPath(p.removeOutdatedRetryFiles)
	p.telemetry.setOutdatedFilesCount(len(outdatedFiles))
	return outdatedFiles, err
}

// RemoveUnknownDomains remove unknown domains.
func (p *FileRemovalPolicy) RemoveUnknownDomains() ([]string, error) {
	files, err := p.forEachDomainPath(func(folderPath string) ([]string, error) {
		if _, found := p.knownDomainFolders[folderPath]; !found {
			return p.removeUnknownDomain(folderPath)
		}
		return nil, nil
	})
	p.telemetry.setFilesFromUnknownDomainCount(len(files))
	return files, err
}

func (p *FileRemovalPolicy) forEachDomainPath(callback func(folderPath string) ([]string, error)) ([]string, error) {
	entries, err := os.ReadDir(p.rootPath)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, domain := range entries {
		if domain.IsDir() {
			folderPath := path.Join(p.rootPath, domain.Name())
			files, err := callback(folderPath)

			if err != nil {
				return nil, err
			}
			paths = append(paths, files...)
		}
	}
	return paths, nil
}

func (p *FileRemovalPolicy) getFolderPathForDomain(domainName string) (string, error) {
	// Use md5 for the folder name as the domainName is an url which can contain invalid charaters for a file path.
	h := md5.New()
	if _, err := io.WriteString(h, domainName); err != nil {
		return "", err
	}
	folder := fmt.Sprintf("%x", h.Sum(nil))

	return path.Join(p.rootPath, folder), nil
}

func (p *FileRemovalPolicy) removeUnknownDomain(folderPath string) ([]string, error) {
	files, err := p.removeRetryFiles(folderPath, func(_ string) bool { return true })

	// Try to remove the folder if it is empty
	_ = os.Remove(folderPath)
	return files, err
}

func (p *FileRemovalPolicy) removeOutdatedRetryFiles(folderPath string) ([]string, error) {
	return p.removeRetryFiles(folderPath, func(filename string) bool {
		modTime, err := filesystem.GetFileModTime(filename)
		if err != nil {
			return false
		}
		return modTime.Before(p.outdatedFileTime)
	})
}

func (p *FileRemovalPolicy) removeRetryFiles(folderPath string, shouldRemove func(string) bool) ([]string, error) {
	files, err := p.getRetryFiles(folderPath)
	if err != nil {
		return nil, err
	}

	var filesRemoved []string
	var errs error
	for _, f := range files {
		if shouldRemove(f) {
			if err = os.Remove(f); err != nil {
				errs = multierror.Append(errs, err)
			} else {
				filesRemoved = append(filesRemoved, f)
			}
		}
	}
	return filesRemoved, errs
}

func (p *FileRemovalPolicy) getRetryFiles(folder string) ([]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && filepath.Ext(entry.Name()) == retryTransactionsExtension {
			files = append(files, path.Join(folder, entry.Name()))
		}
	}
	return files, nil
}
