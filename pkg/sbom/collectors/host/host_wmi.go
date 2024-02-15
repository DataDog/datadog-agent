// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !trivy && windows

package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/yusufpapurcu/wmi"
	"reflect"
	"strings"
	"time"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// Win32_QuickFixEngineering WMI class represents a small system-wide update, commonly referred to as a quick-fix engineering
//
//nolint:revive
type Win32_QuickFixEngineering struct {
	Name        string
	Status      string
	HotFixID    string
	Description string
}

// This is a global variable that will be used to fetch the OS information only once
var osInfo *Win32_OperatingSystem

// Win32_OperatingSystem WMI class represents the properties of a system
//
//nolint:revive
type Win32_OperatingSystem struct {
	Version        string
	Caption        string
	ProductType    uint32
	BuildNumber    string
	OSArchitecture string
	LastBootUpTime time.Time
}

func getOSInfo() (Win32_OperatingSystem, error) {
	if osInfo != nil {
		return *osInfo, nil
	}

	var dst []Win32_OperatingSystem
	q := wmi.CreateQuery(&dst, "")
	err := wmi.Query(q, &dst)
	if err != nil {
		return Win32_OperatingSystem{}, err
	}

	osInfo = &dst[0]

	return dst[0], nil
}

// Report describes a SBOM report along with its marshaler
type Report struct {
	KBS      []Win32_QuickFixEngineering
	hash     []byte
	version  string
	platform string
	family   string
	build    string
	arch     string
}

// ToCycloneDX returns the report as a CycloneDX SBOM
func (r *Report) ToCycloneDX() (*cyclonedxgo.BOM, error) {
	var components []cyclonedxgo.Component

	osProperties := []cyclonedxgo.Property{
		{
			Name:  "Family",
			Value: r.family,
		}, {
			Name:  "Build",
			Value: r.build,
		},
		{
			Name:  "Architecture",
			Value: r.arch,
		},
	}

	windowsComponent := cyclonedxgo.Component{
		Type:       cyclonedxgo.ComponentTypeOS,
		Name:       r.platform,
		Version:    r.version,
		Properties: &osProperties,
	}

	components = append(components, windowsComponent)

	hash := sha256.New()
	for _, kb := range r.KBS {
		components = append(components, cyclonedxgo.Component{
			Name: kb.HotFixID,
			Type: cyclonedxgo.ComponentTypeFile,
		})
		hash.Write([]byte(kb.HotFixID))
	}

	r.hash = hash.Sum(nil)

	return &cyclonedxgo.BOM{
		Components: &components,
	}, nil
}

// ID returns the report identifier
func (r *Report) ID() string {
	return hex.EncodeToString(r.hash)
}

// Collector defines a host collector
type Collector struct {
	version  string
	platform string
	family   string
	build    string
	arch     string
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return nil
}

// Init initialize the host collector
func (c *Collector) Init(_ config.Config) (err error) {
	if c.version, err = winutil.GetWindowsBuildString(); err != nil {
		return err
	}

	tmpOsInfo, err := getOSInfo()
	osInfo = &tmpOsInfo

	// Platform
	c.platform = strings.Trim(osInfo.Caption, " ")

	// Platform Family
	switch osInfo.ProductType {
	case 1:
		c.family = "Standalone Workstation"
	case 2:
		c.family = "Server (Domain Controller)"
	case 3:
		c.family = "Server"
	}

	// Platform Version
	c.build = fmt.Sprintf("%s Build %s", osInfo.Version, osInfo.BuildNumber)

	// Platform Architecture
	c.arch = osInfo.OSArchitecture
	return err
}

// Scan performs a scan
func (c *Collector) Scan(_ context.Context, request sbom.ScanRequest, _ sbom.ScanOptions) sbom.ScanResult {
	hostScanRequest, ok := request.(*ScanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector '%s'", reflect.TypeOf(request), collectorName)}
	}
	log.Infof("host scan request [%v]", hostScanRequest.ID())

	report := Report{version: c.version, platform: c.platform, family: c.family, build: c.build, arch: c.arch}
	q := wmi.CreateQuery(&report.KBS, "")
	err := wmi.Query(q, &report.KBS)
	if err != nil {
		return sbom.ScanResult{
			Error: err,
		}
	}

	return sbom.ScanResult{
		Error:  err,
		Report: &report,
	}
}
