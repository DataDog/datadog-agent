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
	"reflect"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/gopsutil/host"

	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/yusufpapurcu/wmi"
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

// Report describes a SBOM report along with its marshaler
type Report struct {
	KBS      []Win32_QuickFixEngineering
	hash     []byte
	version  string
	platform string
	family   string
	build    string
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

	resChan chan sbom.ScanResult
	opts    sbom.ScanOptions

	closed bool
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return nil
}

// Init initialize the host collector
func (c *Collector) Init(cfg config.Component, wmeta optional.Option[workloadmeta.Component]) (err error) {
	if c.version, err = winutil.GetWindowsBuildString(); err != nil {
		return err
	}

	c.platform, c.family, c.build, err = host.PlatformInformation()
	return err
}

// Scan performs a scan
func (c *Collector) Scan(_ context.Context, request sbom.ScanRequest, _ sbom.ScanOptions) sbom.ScanResult {
	hostScanRequest, ok := request.(*sbom.ScanRequest)
	if !ok {
		return sbom.ScanResult{Error: fmt.Errorf("invalid request type '%s' for collector 'host'", reflect.TypeOf(request))}
	}
	log.Infof("host scan request [%v]", hostScanRequest.ID())

	report := Report{version: c.version, platform: c.platform, family: c.family, build: c.build}
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
