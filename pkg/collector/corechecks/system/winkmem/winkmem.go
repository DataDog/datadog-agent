// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.
//go:build windows

package winkmem

import (
	"encoding/binary"
	"fmt"
	"sort"
	"unsafe"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
)

const (
	kmemCheckName = "winkmem"

	// KMemDefaultTopNum is the default number of kernel memory tags to return
	KMemDefaultTopNum = 10

	systemPoolTagInformation          = 0x16
	cEmptySystemInformationStructSize = 48
	cEmptyPoolTagStructSize           = 40
	statusInformationMismatchError    = 0xC0000004
)

var (
	modntdll                     = windows.NewLazyDLL("ntdll.dll")
	procNtQuerySystemInformation = modntdll.NewProc("NtQuerySystemInformation")
)

// Config is the configuration options for this check
// it is exported so that the yaml parser can read it.
type Config struct {
	TopPagedBytes                int      `yaml:"top_paged_bytes"`
	TopNonPagedBytes             int      `yaml:"top_non_paged_bytes"`
	TopPagedAllocsOutstanding    int      `yaml:"top_paged_allocs_outstanding"`
	TopNonPagedAllocsOutstanding int      `yaml:"top_non_paged_allocs_outstanding"`
	SpecificTags                 []string `yaml:"extra_tags"`
}

// KMemCheck is the agent object for this check.
type KMemCheck struct {
	core.CheckBase
	config Config
}

func init() {
	core.RegisterCheck(kmemCheckName, winkmemFactory)
}

func winkmemFactory() check.Check {
	return &KMemCheck{
		CheckBase: core.NewCheckBase(kmemCheckName),
	}
}

/*
configuration:

init_config:
  topPagedBytes: 10  // indicates record the top 10 paged-pool byte users
  topNonPagedBytes: 9 // indicates record the top 9 non-paged pool byte users
  topPagedAllocsOutstanding: 8 // indicates record the top 8 by number of non-freed allocs
  topNonPagedAllocsOutstanding: 7 // indicates record the top 7 by number of non freed allocs
  allocTags:
    - HTab      // indicates record the allocs with memtag `HTab` regardless of whether it's in the top N above
*/

// Configure is called to configure the object prior to the first run
func (w *KMemCheck) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	// check to make sure the function is actually there, so we can fail gracefully
	// if it's not
	if err := modntdll.Load(); err != nil {
		return err
	}
	if err := procNtQuerySystemInformation.Find(); err != nil {
		return err
	}

	if err := w.CommonConfigure(integrationConfigDigest, initConfig, data, source); err != nil {
		return err
	}
	cf := Config{
		TopNonPagedBytes:             KMemDefaultTopNum,
		TopPagedBytes:                KMemDefaultTopNum,
		TopPagedAllocsOutstanding:    KMemDefaultTopNum,
		TopNonPagedAllocsOutstanding: KMemDefaultTopNum,
	}
	err := yaml.Unmarshal(initConfig, &cf)
	if err != nil {
		return err
	}

	w.config = cf
	log.Infof("Winkmem config %v", w.config)
	return nil
}

// Run executes the check
func (w *KMemCheck) Run() error {
	sender, err := w.GetSender()
	if err != nil {
		return err
	}
	spi, err := getpoolinfo()
	if err != nil {
		return err
	}
	// make a map to keep track of the indexes we actually want
	// to send up (so we don't double count them)
	tagmap := make(map[int]bool)
	for i := 0; i < w.config.TopPagedBytes; i++ {
		tagmap[spi.pagedPoolBytes[i].Value] = true
	}
	for i := 0; i < w.config.TopNonPagedBytes; i++ {
		tagmap[spi.nonPagedPoolBytes[i].Value] = true
	}
	for i := 0; i < w.config.TopPagedAllocsOutstanding; i++ {
		tagmap[spi.pagedPoolAllocsOutstanding[i].Value] = true
	}
	for i := 0; i < w.config.TopNonPagedAllocsOutstanding; i++ {
		tagmap[spi.nonPagedPoolAllocsOutstanding[i].Value] = true
	}
	if len(w.config.SpecificTags) > 0 {
		for _, ktag := range w.config.SpecificTags {
			tagmap[spi.byKey[ktag]] = true
		}
	}

	// now we have a list of indexes we actually want to store.  Walk them,
	// and do it
	for k, v := range tagmap {
		// double sanity check, but should always be true
		if v {
			tags := []string{}
			pti := spi.spti.poolTags[k]

			tags = append(tags, "kmemtag:"+string(pti.tag[:]))
			log.Debugf("Logging mem tag %v", tags)

			sender.Gauge("winkmem.paged_pool_bytes", float64(pti.pagedUsed), "", tags)
			sender.Gauge("winkmem.nonpaged_pool_bytes", float64(pti.nonPagedUsed), "", tags)
			sender.Gauge("winkmem.paged_allocs_outstanding", float64(pti.pagedAllocs-pti.pagedFrees), "", tags)
			sender.Gauge("winkmem.nonpaged_allocs_outstanding", float64(pti.nonPagedAllocs-pti.nonPagedFrees), "", tags)
		}
	}
	sender.Commit()
	log.Debugf("Logged %v entries", len(tagmap))
	return nil
}

type systemPooltag struct {
	tag            [4]uint8
	pagedAllocs    uint32
	pagedFrees     uint32
	pagedUsed      uint64
	nonPagedAllocs uint32
	nonPagedFrees  uint32
	nonPagedUsed   uint64
}
type systemPooltagInformation struct {
	count    uint32
	_        uint32 // padding
	poolTags []systemPooltag
}

type systemPoolInformation struct {
	spti                          systemPooltagInformation
	byKey                         map[string]int
	pagedPoolBytes                sortKeyList
	nonPagedPoolBytes             sortKeyList
	pagedPoolAllocsOutstanding    sortKeyList
	nonPagedPoolAllocsOutstanding sortKeyList
}
type sortKey struct {
	Key   uint64 // numerical value (i.e. allocs, frees, etc)
	Value int    // index into pooltags array
}
type sortKeyList []sortKey

func (p sortKeyList) Len() int { return len(p) }
func (p sortKeyList) Swap(i, j int) {
	p[j], p[i] = p[i], p[j]
}
func (p sortKeyList) Less(i, j int) bool { return p[i].Key < p[j].Key }

func getpoolinfo() (*systemPoolInformation, error) {
	firstbuffer := make([]uint8, cEmptySystemInformationStructSize) // magic size of empty structure in C land
	var returnlen uint32
	r, _, _ := procNtQuerySystemInformation.Call(uintptr(systemPoolTagInformation),
		uintptr(unsafe.Pointer(&firstbuffer[0])),
		48,
		uintptr(unsafe.Pointer(&returnlen)))
	if r != statusInformationMismatchError {
		return nil, fmt.Errorf("Expected STATUS_INFO_LENGTH_MISMATCH, got %v", r)
	}

	fullbuffer := make([]uint8, returnlen)
	r, _, _ = procNtQuerySystemInformation.Call(uintptr(systemPoolTagInformation),
		uintptr(unsafe.Pointer(&fullbuffer[0])),
		uintptr(returnlen),
		uintptr(unsafe.Pointer(&returnlen)))
	if r != 0 {
		return nil, fmt.Errorf("Unexpected error getting system info %v", r)
	}
	spi := &systemPoolInformation{
		byKey: make(map[string]int),
	}
	/* In C, the (undocumented) C structure is
		struct SYSTEM_POOLTAG_INFORMATION {
	    	ULONG Count;
	    	SYSTEM_POOLTAG TagInfo[1];
		};
		So the `count` field is the first 4 bytes
	*/

	spi.spti.count = binary.LittleEndian.Uint32(fullbuffer[:4])

	for i := 0; i < int(spi.spti.count); i++ {
		// in C, the (undocumented) C structure is as above.  a 32 bit int
		// which is the count of the number of structures, followed by an array
		// of the actual structures.  However, the structure is 64-bit byte
		// aligned, so the array actually starts 8 bytes off the start of the
		// buffer, not 4.  (hence the 8+ in the magic calculation)
		// Then, the actual structure is 40 bytes long, so the index into the buffer
		// becomes the 8 bytes plus the number of structs we want to skip.
		pt := (*systemPooltag)(unsafe.Pointer(&fullbuffer[8+(cEmptyPoolTagStructSize*i)]))

		tagstr := string(pt.tag[:])
		spi.byKey[tagstr] = i
		spi.pagedPoolBytes = append(spi.pagedPoolBytes, sortKey{pt.pagedUsed, i})
		spi.nonPagedPoolBytes = append(spi.nonPagedPoolBytes, sortKey{pt.nonPagedUsed, i})
		spi.pagedPoolAllocsOutstanding = append(spi.pagedPoolAllocsOutstanding, sortKey{uint64(pt.pagedAllocs) - uint64(pt.pagedFrees), i})
		spi.nonPagedPoolAllocsOutstanding = append(spi.nonPagedPoolAllocsOutstanding, sortKey{uint64(pt.nonPagedAllocs) - uint64(pt.nonPagedFrees), i})
		spi.spti.poolTags = append(spi.spti.poolTags, *pt)
	}
	sort.Sort(sort.Reverse(spi.pagedPoolBytes))
	sort.Sort(sort.Reverse(spi.nonPagedPoolBytes))
	sort.Sort(sort.Reverse(spi.pagedPoolAllocsOutstanding))
	sort.Sort(sort.Reverse(spi.nonPagedPoolAllocsOutstanding))

	return spi, nil
}
