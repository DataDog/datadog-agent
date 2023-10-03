// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/textproto"
	"os"

	rpmdb "github.com/knqyf263/go-rpmdb/pkg"
	"golang.org/x/exp/slices"
)

type packageInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

func packageNameMatch(names []string, pkgName string) bool {
	return slices.Contains(names, pkgName)
}

// scanBlock is a utility function that can be used to scan through text files
// that chunk using two-lined separators.
func scanBlock(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if n := bytes.Index(data, []byte("\n\n")); n != -1 {
		return n + 2, data[0 : n+1], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return
}

func findApkPackage(path string, names []string) *packageInfo {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(scanBlock)
	for scanner.Scan() {
		var p packageInfo
		lines := bytes.Split(scanner.Bytes(), []byte("\n"))
		for _, line := range lines {
			if len(line) < 2 {
				continue
			}
			key, value := line[:2], line[2:]
			switch string(key) {
			case "P:":
				p.Name = string(value)
			case "V:":
				p.Version = string(value)
			case "A:":
				p.Arch = string(value)
			}
		}
		if p.Name == "" {
			continue
		}
		if packageNameMatch(names, p.Name) {
			return &p
		}
	}
	return nil
}

func findDpkgPackage(path string, names []string) *packageInfo {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// The dpkg status databases is composed of blocks of "MIME" headers
	// separated by two newlines.
	//
	// Fast path of this scan: as we chunk the database in blocks, we check in
	// each block for a matching "Package: $name" substring in the block. Only
	// if we find such a substring do we involve the more elaborated textproto
	// MIME reader to fully parse the MIME block.
	scanner := bufio.NewScanner(f)
	scanner.Split(scanBlock)

	pkgNamePrefix := []byte("Package:")
	for scanner.Scan() {
		chunk := scanner.Bytes()
		lines := bytes.Split(chunk, []byte("\n"))
		for _, line := range lines {
			if !bytes.HasPrefix(line, pkgNamePrefix) {
				continue
			}
			pkgName := string(bytes.TrimSpace(line[len(pkgNamePrefix):]))
			if !packageNameMatch(names, pkgName) {
				break
			}
			reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(chunk)))
			header, err := reader.ReadMIMEHeader()
			if err != nil && !errors.Is(err, io.EOF) {
				return nil
			}
			name := header.Get("Package")
			if name != pkgName { // fast path miss
				continue
			}
			return &packageInfo{
				Name:    name,
				Version: header.Get("Version"),
				Arch:    header.Get("Architecture"),
			}
		}
	}

	return nil
}

func findRpmPackage(path string, names []string) *packageInfo {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	db, err := rpmdb.Open(path)
	if err != nil {
		return nil
	}
	pkgs, err := db.ListPackages()
	if err != nil {
		return nil
	}
	for _, pkg := range pkgs {
		if !packageNameMatch(names, pkg.Name) {
			return &packageInfo{
				Name:    pkg.Name,
				Version: pkg.Version,
				Arch:    pkg.Arch,
			}
		}
	}
	return nil
}
