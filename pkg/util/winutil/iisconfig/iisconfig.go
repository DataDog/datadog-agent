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

// iisEnvVarOpKind identifies which collection directive a parsed entry
// came from inside an <environmentVariables> block.
type iisEnvVarOpKind int

const (
	iisEnvVarOpAdd iisEnvVarOpKind = iota
	iisEnvVarOpRemove
	iisEnvVarOpClear
)

// iisEnvVarOp is one parsed <add>/<remove>/<clear> directive. Ops are
// stored in document order so that order-dependent inheritance behavior
// (e.g. <add name=X/><clear/>) is preserved.
type iisEnvVarOp struct {
	kind  iisEnvVarOpKind
	name  string
	value string
}

type iisEnvironmentVariables struct {
	XMLName xml.Name      `xml:"environmentVariables"`
	Ops     []iisEnvVarOp `xml:"-"`
}

// UnmarshalXML walks the <environmentVariables> children in document order
// and records each <add>/<remove>/<clear> directive. encoding/xml's default
// decoder collapses children into per-tag slices, which loses the ordering
// IIS uses to evaluate these collections.
func (e *iisEnvironmentVariables) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	e.XMLName = start.Name
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch se := tok.(type) {
		case xml.StartElement:
			op := iisEnvVarOp{}
			switch se.Name.Local {
			case "add":
				op.kind = iisEnvVarOpAdd
				for _, a := range se.Attr {
					switch a.Name.Local {
					case "name":
						op.name = a.Value
					case "value":
						op.value = a.Value
					}
				}
				e.Ops = append(e.Ops, op)
			case "remove":
				op.kind = iisEnvVarOpRemove
				for _, a := range se.Attr {
					if a.Name.Local == "name" {
						op.name = a.Value
					}
				}
				e.Ops = append(e.Ops, op)
			case "clear":
				e.Ops = append(e.Ops, iisEnvVarOp{kind: iisEnvVarOpClear})
			}
			if err := d.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			if se.Name == start.Name {
				return nil
			}
		}
	}
}

type iisApplication struct {
	XMLName     xml.Name              `xml:"application"`
	Path        string                `xml:"path,attr"`
	AppPool     string                `xml:"applicationPool,attr"`
	VirtualDirs []iisVirtualDirectory `xml:"virtualDirectory"`
}

// iisApplicationDefaults captures <applicationDefaults> elements that supply
// the inherited applicationPool for <application> entries that omit the
// applicationPool attribute. Appears under both <sites> (global) and each
// <site> (per-site); per-site wins over global.
type iisApplicationDefaults struct {
	XMLName xml.Name `xml:"applicationDefaults"`
	AppPool string   `xml:"applicationPool,attr"`
}

type iisSite struct {
	Name         string                 `xml:"name,attr"`
	SiteID       string                 `xml:"id,attr"`
	Applications []iisApplication       `xml:"application"`
	Bindings     []iisBinding           `xml:"bindings>binding"`
	AppDefaults  iisApplicationDefaults `xml:"applicationDefaults"`
}
type iisApplicationPool struct {
	XMLName xml.Name                `xml:"add"`
	Name    string                  `xml:"name,attr"`
	EnvVars iisEnvironmentVariables `xml:"environmentVariables"`
}
type iisApplicationPoolDefaults struct {
	XMLName xml.Name                `xml:"applicationPoolDefaults"`
	EnvVars iisEnvironmentVariables `xml:"environmentVariables"`
}
type iisApplicationPools struct {
	XMLName  xml.Name                   `xml:"applicationPools"`
	Defaults iisApplicationPoolDefaults `xml:"applicationPoolDefaults"`
	Pools    []iisApplicationPool       `xml:"add"`
}
type iisSystemApplicationHost struct {
	XMLName          xml.Name               `xml:"system.applicationHost"`
	Sites            []iisSite              `xml:"sites>site"`
	SitesAppDefaults iisApplicationDefaults `xml:"sites>applicationDefaults"`
	ApplicationPools iisApplicationPools    `xml:"applicationPools"`
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
