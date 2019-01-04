// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build android

package androidasset

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	//"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/mobile/asset"
	yaml "gopkg.in/yaml.v2"
)

var (
	assetFileName = "directory_manifest.yaml"
)

/*
 There is an outstanding PR for handling the listing of assets for the go/mobile
 project.  However, it will not be merged in a timely enough fashion.

 This function presents a workaround, in which we'll provide a manifest of files
 that we're interested in, and return the list of files as a slice of os.FileInfos
 so that the return is as close to the expected as possible, and requires as little
 change in the main code as possible.
*/

// ReadFile reads an entire asset file into a byte slice.
func ReadFile(name string) ([]byte, error) {
	f, errOpen := asset.Open(name)
	//var f *os.File
	//var errOpen error

	if errOpen != nil {
		log.Printf("asset.Open %v", errOpen)
		return nil, errOpen
	}
	log.Printf("asset.open succeeded")
	defer f.Close()
	buf, errRead := ioutil.ReadAll(f)
	if errRead != nil {
		return nil, errRead
	}
	log.Printf("ReadFile len %d", len(buf))
	return buf, nil
}

// A fileStat is the implementation of FileInfo returned by Stat and Lstat.
type assetFileStat struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	sys     interface{}
}

func (fs *assetFileStat) Size() int64        { return fs.size }
func (fs *assetFileStat) Mode() os.FileMode  { return fs.mode }
func (fs *assetFileStat) ModTime() time.Time { return fs.modTime }
func (fs *assetFileStat) Sys() interface{}   { return fs.sys }
func (fs *assetFileStat) IsDir() bool        { return false }
func (fs *assetFileStat) Name() string       { return fs.name }

type directoryManifest struct {
	Files []string `yaml:"files"`
}

// ReadDir returns a list of files present in an asset directory.
func ReadDir(dirname string) ([]os.FileInfo, error) {
	log.Printf("androidasset.ReadDir() %s", dirname)
	var assetFile string
	if dirname == "" || dirname == "." {
		assetFile = assetFileName
	} else {
		assetFile = filepath.Join(dirname, assetFileName)
	}
	var yamlFile []byte
	var err error
	log.Printf("androidasset.ReadFile() %s", assetFile)
	if yamlFile, err = ReadFile(assetFile); err != nil {
		log.Printf("Error loading asset file %s %v", assetFile, err)
		return nil, err
	}
	if len(yamlFile) == 0 {
		log.Printf("empty yaml file")
		return nil, fmt.Errorf("File not found")
	}
	log.Printf("dir: %s", string(yamlFile[:]))
	ret := make([]os.FileInfo, 0)
	manifest := &directoryManifest{}
	err = yaml.Unmarshal(yamlFile, manifest)
	if err != nil {
		return nil, err
	}
	for _, f := range manifest.Files {
		ret = append(ret, &assetFileStat{
			name:    f,
			size:    1, // don't know the size, for now...
			mode:    0444,
			modTime: time.Now(),
			sys:     nil,
		})

	}
	return ret, nil

}
