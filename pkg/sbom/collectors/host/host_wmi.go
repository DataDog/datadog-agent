// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !trivy && windows && wmi

package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"

	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/gopsutil/host"

	host2 "github.com/shirou/gopsutil/v4/host"
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
	arch     string
}

// ToCycloneDX returns the report as a CycloneDX SBOM
func (r *Report) ToCycloneDX() *cyclonedx_v1_4.Bom {
	var components []*cyclonedx_v1_4.Component

	osProperties := []*cyclonedx_v1_4.Property{
		{
			Name:  "Platform",
			Value: pointer.Ptr(r.platform),
		},
		{
			Name:  "Family",
			Value: pointer.Ptr(r.family),
		}, {
			Name:  "Build",
			Value: pointer.Ptr(r.build),
		},
		{
			Name:  "Architecture",
			Value: pointer.Ptr(r.arch),
		},
	}

	windowsComponent := &cyclonedx_v1_4.Component{
		Type:       cyclonedx_v1_4.Classification_CLASSIFICATION_OPERATING_SYSTEM,
		Name:       "windows",
		Version:    r.version,
		Properties: osProperties,
	}

	components = append(components, windowsComponent)

	hash := sha256.New()
	for _, kb := range r.KBS {
		components = append(components, &cyclonedx_v1_4.Component{
			Name: kb.HotFixID,
			Type: cyclonedx_v1_4.Classification_CLASSIFICATION_FILE,
		})
		hash.Write([]byte(kb.HotFixID))
	}

	r.hash = hash.Sum(nil)

	return &cyclonedx_v1_4.Bom{
		Components: components,
	}
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
	arch   string
}

// CleanCache cleans the cache
func (c *Collector) CleanCache() error {
	return nil
}

// Init initialize the host collector
func (c *Collector) Init(_ config.Component, _ option.Option[workloadmeta.Component]) (err error) {
	if c.version, err = winutil.GetWindowsBuildString(); err != nil {
		return err
	}

	c.platform, c.family, c.build, err = host.PlatformInformation()
	if err != nil {
		return err
	}

	c.arch, err = host2.KernelArch()
	if err != nil {
		return err
	}
	return err
}

// Scan performs a scan
func (c *Collector) Scan(_ context.Context, _ sbom.ScanRequest) sbom.ScanResult {
	report := Report{version: c.version, platform: c.platform, family: c.family, build: c.build, arch: c.arch}
	q := wmi.CreateQuery(&report.KBS, "")
	err := wmi.Query(q, &report.KBS)
	if err != nil {
		return sbom.ScanResult{
			GenerationMethod: "wmi",
			Error:            err,
		}
	}

	return sbom.ScanResult{
		Error:            err,
		Report:           &report,
		GenerationMethod: "wmi",
	}
}
