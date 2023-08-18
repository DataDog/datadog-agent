// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package winutil

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
}

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

func (iiscfg *DynamicIISConfig) Start() error {
	if iiscfg == nil {
		return fmt.Errorf("Null config")
	}
	// set the filepath
	err := iiscfg.watcher.Add(iiscfg.path)
	if err != nil {
		return err
	}
	err = iiscfg.readXmlConfig()
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
					_ = iiscfg.readXmlConfig()
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

func (iiscfg *DynamicIISConfig) Stop() {
	iiscfg.stopChannel <- true
	iiscfg.wg.Wait()
	return
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
	Name        string `xml:"name,attr"`
	SiteID      string `xml:"id,attr"`
	Application iisApplication
	Bindings    []iisBinding `xml:"bindings>binding"`
}
type iisSystemApplicationHost struct {
	XMLName xml.Name  `xml:"system.applicationHost"`
	Sites   []iisSite `xml:"sites>site"`
}
type iisConfiguration struct {
	XMLName         xml.Name `xml:"configuration"`
	ApplicationHost iisSystemApplicationHost
}

func (iiscfg *DynamicIISConfig) readXmlConfig() error {
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
	iiscfg.mux.Lock()
	defer iiscfg.mux.Unlock()
	iiscfg.xmlcfg = &newcfg
	iiscfg.siteIDToName = idmap
	return nil
}

func (iiscfg *DynamicIISConfig) GetSiteNameFromId(id uint32) string {
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
