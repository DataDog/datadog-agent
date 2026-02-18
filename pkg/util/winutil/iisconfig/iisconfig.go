// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package iisconfig

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/fsnotify/fsnotify"
)

var (
	// make global so that we can override for tests.
	iisCfgPath = filepath.Join(os.Getenv("windir"), "System32", "inetsrv", "config", "applicationHost.config")
)

// DynamicIISConfig is an object that will watch the IIS configuration for
// changes, and reload the configuration when it changes.  It provides additional
// methods for getting specific configuration items
type DynamicIISConfig struct {
	watcher      *fsnotify.Watcher
	path         string
	wg           sync.WaitGroup
	mux          sync.Mutex
	stopChannel  chan bool
	xmlcfg       *iisConfiguration
	siteIDToName map[uint32]string
	pathTrees    map[uint32]*pathTreeEntry
}

// NewDynamicIISConfig creates a new DynamicIISConfig
func NewDynamicIISConfig() (*DynamicIISConfig, error) {
	iiscfg := &DynamicIISConfig{
		stopChannel: make(chan bool),
	}
	var err error

	iiscfg.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// check for existence
	_, err = os.Stat(iisCfgPath)
	if os.IsNotExist(err) {
		return nil, err
	} else if err != nil {
		return nil, err
	}
	iiscfg.path = iisCfgPath
	return iiscfg, nil
}

// Start config watcher
func (iiscfg *DynamicIISConfig) Start() error {
	if iiscfg == nil {
		return errors.New("Null config")
	}
	// set the filepath
	err := iiscfg.watcher.Add(iiscfg.path)
	if err != nil {
		return err
	}
	err = iiscfg.readXMLConfig()
	if err != nil {
		return err
	}
	iiscfg.wg.Add(1)
	go func() {
		defer iiscfg.wg.Done()
		for {
			select {
			case event := <-iiscfg.watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					_ = iiscfg.readXMLConfig()
				}
			case err = <-iiscfg.watcher.Errors:
				return
			case <-iiscfg.stopChannel:
				return
			}
		}

	}()
	return nil
}

// Stop config watcher
func (iiscfg *DynamicIISConfig) Stop() {
	iiscfg.stopChannel <- true
	iiscfg.wg.Wait()
}

type iisVirtualDirectory struct {
	Path         string `xml:"path,attr"`
	PhysicalPath string `xml:"physicalPath,attr"`
}
type iisBinding struct {
	Protocol    string `xml:"protocol,attr"`
	BindingInfo string `xml:"bindingInformation,attr"`
}
type iisApplication struct {
	XMLName     xml.Name              `xml:"application"`
	Path        string                `xml:"path,attr"`
	AppPool     string                `xml:"applicationPool,attr"`
	VirtualDirs []iisVirtualDirectory `xml:"virtualDirectory"`
}
type iisSite struct {
	Name         string           `xml:"name,attr"`
	SiteID       string           `xml:"id,attr"`
	Applications []iisApplication `xml:"application"`
	Bindings     []iisBinding     `xml:"bindings>binding"`
}
type iisSystemApplicationHost struct {
	XMLName xml.Name  `xml:"system.applicationHost"`
	Sites   []iisSite `xml:"sites>site"`
}

type iisConfiguration struct {
	XMLName         xml.Name `xml:"configuration"`
	ApplicationHost iisSystemApplicationHost
	AppSettings     iisAppSettings
}

func (iiscfg *DynamicIISConfig) readXMLConfig() error {
	var newcfg iisConfiguration
	f, err := os.ReadFile(iiscfg.path)
	if err != nil {
		return err
	}
	err = xml.Unmarshal(f, &newcfg)
	if err != nil {
		return err
	}
	idmap := make(map[uint32]string)

	for _, site := range newcfg.ApplicationHost.Sites {
		id, err := strconv.Atoi(site.SiteID)
		if err != nil {
			return err
		}
		idmap[uint32(id)] = site.Name
	}

	pt := buildPathTagTree(&newcfg)
	iiscfg.mux.Lock()
	defer iiscfg.mux.Unlock()
	iiscfg.xmlcfg = &newcfg
	iiscfg.siteIDToName = idmap
	iiscfg.pathTrees = pt
	return nil
}

// GetSiteNameFromID looks up a site name by its site ID
func (iiscfg *DynamicIISConfig) GetSiteNameFromID(id uint32) string {
	if iiscfg == nil {
		log.Warnf("GetSiteNameFromId %d NIL", id)
		return ""
	}
	var val string
	var ok bool
	iiscfg.mux.Lock()
	defer iiscfg.mux.Unlock()
	if val, ok = iiscfg.siteIDToName[id]; !ok {
		return ""
	}
	return val
}

// GetApplicationPath returns the IIS application path that handles the given URL path
func (iiscfg *DynamicIISConfig) GetApplicationPath(siteID uint32, urlpath string) string {
	if iiscfg == nil {
		return ""
	}
	iiscfg.mux.Lock()
	defer iiscfg.mux.Unlock()

	if iiscfg.xmlcfg == nil {
		return ""
	}

	// Convert siteID to string once for comparison
	siteIDStr := strconv.FormatUint(uint64(siteID), 10)

	// Convert urlpath to lowercase for case-insensitive comparison (Windows paths are case-insensitive)
	urlpathLower := strings.ToLower(urlpath)

	// Find the matching site and iterate applications to find longest match
	for _, site := range iiscfg.xmlcfg.ApplicationHost.Sites {
		if site.SiteID != siteIDStr {
			continue
		}

		// Find the longest matching application path
		longestMatch := "/"
		for _, app := range site.Applications {
			appPathLower := strings.ToLower(app.Path)
			if urlpathLower == appPathLower {
				return app.Path
			}
			// Check if urlpath starts with app.Path and has proper boundary
			// (either app.Path is "/" or next char is "/")
			if strings.HasPrefix(urlpathLower, appPathLower) &&
				(appPathLower == "/" || (len(urlpathLower) > len(appPathLower) && urlpathLower[len(appPathLower)] == '/')) {
				if len(app.Path) > len(longestMatch) {
					longestMatch = app.Path
				}
			}
		}
		return longestMatch
	}
	return ""
}
