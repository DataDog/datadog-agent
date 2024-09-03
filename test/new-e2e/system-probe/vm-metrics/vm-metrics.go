// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package systemprobe is sets up the remote testing environment for system-probe using the Kernel Matrix Testing framework
package main

import (
	"flag"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
)

const kmtMicroVmsPrefix = "kmt.microvm."

var distrosMatch = map[string]*regexp.Regexp{
	"ubuntu":   regexp.MustCompile(`-(ubuntu_[\.,\d]{1,5}).*-`),
	"fedora":   regexp.MustCompile(`-(fedora_[\.,\d]{1,5}).*-`),
	"debian":   regexp.MustCompile(`-(debian_[\.,\d]{1,5}).*-`),
	"centos":   regexp.MustCompile(`-(centos_[\.,\d]{1,5}).*-`),
	"amazon":   regexp.MustCompile(`-(amazon_[\.,\d]{1,5}).*-`),
	"rocky":    regexp.MustCompile(`-(rocky_[\.,\d]{1,5}).*-`),
	"oracle":   regexp.MustCompile(`-(oracle_[\.,\d]{1,5}).*-`),
	"opensuse": regexp.MustCompile(`-(opensuse_[\.,\d]{1,5}).*-`),
	"suse":     regexp.MustCompile(`-(suse_[\.,\d]{1,5}).*-`),
}

var memStatTagToName = map[libvirt.DomainMemoryStatTags]string{
	libvirt.DomainMemoryStatSwapIn:        "swap_in_bytes",
	libvirt.DomainMemoryStatSwapOut:       "swap_out_bytes",
	libvirt.DomainMemoryStatMajorFault:    "major_pagefault",
	libvirt.DomainMemoryStatAvailable:     "memory_available_bytes",
	libvirt.DomainMemoryStatActualBalloon: "memory_actual_balloon_bytes",
	libvirt.DomainMemoryStatRss:           "memory_rss_bytes",
}

type libvirtInterface interface {
	ConnectListAllDomains(int32, libvirt.ConnectListAllDomainsFlags) ([]libvirt.Domain, uint32, error)
	DomainMemoryStats(libvirt.Domain, uint32, uint32) ([]libvirt.DomainMemoryStat, error)
}

type libvirtExporter struct {
	libvirt      libvirtInterface
	statsdClient statsd.ClientInterface
}

func newLibvirtExporter(l libvirtInterface, client statsd.ClientInterface) *libvirtExporter {
	return &libvirtExporter{
		libvirt:      l,
		statsdClient: client,
	}
}

func (l *libvirtExporter) collect() ([]*domainMetrics, error) {
	return collectLibvirtMetrics(l.libvirt)
}

func (l *libvirtExporter) submit(metrics []*domainMetrics) error {
	for _, dm := range metrics {
		for _, m := range dm.metrics {
			if err := l.statsdClient.Gauge(kmtMicroVmsPrefix+m.name, float64(m.value), m.tags, 1); err != nil {
				return fmt.Errorf("error sending metric: %w", err)
			}
		}
	}
	if err := l.statsdClient.Flush(); err != nil {
		return fmt.Errorf("failed to flush client: %w", err)
	}

	return nil
}

type statsdMetric struct {
	name  string
	value uint64
	tags  []string
}

type domainMetrics struct {
	osID    string
	metrics []statsdMetric

	libvirtDomain libvirt.Domain
}

func (d *domainMetrics) addMetric(name string, value uint64, tags []string) {
	d.metrics = append(d.metrics, statsdMetric{
		name:  name,
		value: value,
		tags:  tags,
	})
}

func kbToBytes(kb uint64) uint64 {
	return kb * 1024
}

func (d *domainMetrics) collectDomainMemoryStatInfo(l libvirtInterface) error {
	memStats, err := l.DomainMemoryStats(d.libvirtDomain, uint32(libvirt.DomainMemoryStatNr), 0)
	if err != nil {
		return fmt.Errorf("failed to get memory stats: %w", err)
	}

	tags := []string{fmt.Sprintf("os:%s", d.osID)}
	for _, stat := range memStats {
		if statString, ok := memStatTagToName[libvirt.DomainMemoryStatTags(stat.Tag)]; ok {
			if stat.Tag == int32(libvirt.DomainMemoryStatMajorFault) {
				d.addMetric(statString, stat.Val, tags)
			} else {
				d.addMetric(statString, kbToBytes(stat.Val), tags)
			}
		}
	}

	return nil
}

func collectLibvirtMetrics(l libvirtInterface) ([]*domainMetrics, error) {
	var dMetrics []*domainMetrics

	domains, _, err := l.ConnectListAllDomains(1, libvirt.ConnectListDomainsActive)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	for _, d := range domains {
		osID := parseOSInformation(d.Name)
		if osID == "" {
			continue
		}

		dMetrics = append(dMetrics, &domainMetrics{
			osID:          osID,
			libvirtDomain: d,
		})
	}

	for _, d := range dMetrics {
		if err := d.collectDomainMemoryStatInfo(l); err != nil {
			return nil, fmt.Errorf("failed to collect memory stats for domain %s: %w", d.osID, err)
		}
	}

	return dMetrics, nil
}

func parseOSInformation(name string) string {
	for _, distro := range distrosMatch {
		if match := distro.FindStringSubmatch(name); match != nil {
			return match[1]
		}
	}

	return ""
}

type tagsList []string

func (t *tagsList) String() string {
	return fmt.Sprintf("%v", *t)
}

func (t *tagsList) Set(value string) error {
	*t = append(*t, value)
	return nil
}

func main() {
	var globalTags tagsList

	statsdPort := flag.String("statsd-port", "8125", "Statsd port")
	statsdHost := flag.String("statsd-host", "127.0.0.1", "Statsd host")
	collectionInterval := flag.Duration("interval", time.Second*20, "interval for collecting vm stats")
	libvirtDaemonURI := flag.String("libvirt-uri", "", "libvirt daemon URI")
	flag.Var(&globalTags, "tag", "global tags to set")
	flag.Parse()

	dialer := dialers.NewLocal(dialers.WithSocket(*libvirtDaemonURI), dialers.WithLocalTimeout((5 * time.Second)))
	l := libvirt.NewWithDialer(dialer)
	if err := l.ConnectToURI(libvirt.QEMUSystem); err != nil {
		log.Fatal(fmt.Sprintf("failed to connect to libvirt: %v", err))
	}
	defer func() {
		if err := l.Disconnect(); err != nil {
			log.Printf("failed to disconnect: %v", err)
		}
	}()

	fmt.Println(globalTags)
	dogstatsd_client, err := statsd.New(fmt.Sprintf("%s:%s", *statsdHost, *statsdPort), statsd.WithTags(globalTags))
	if err != nil {
		log.Fatal(err)
	}

	lexporter := newLibvirtExporter(l, dogstatsd_client)

	for range time.Tick(*collectionInterval) {
		metrics, err := lexporter.collect()
		if err != nil {
			log.Fatal(err)
		}

		if err := lexporter.submit(metrics); err != nil {
			log.Fatal(err)
		}
	}
}
