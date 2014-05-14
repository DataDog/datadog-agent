package memory

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
)

type Memory struct{}

const name = "memory"

func (self *Memory) Name() string {
	return name
}

func (self *Memory) Collect() (result interface{}, err error) {
	result, err = getMemoryInfo()
	return
}

var memMap = map[string]string{
	"MemTotal":  "total",
	"SwapTotal": "swap_total",
	// "MemFree":      "free",
	// "Buffers":      "buffers",
	// "Cached":       "cached",
	// "Active":       "active",
	// "Inactive":     "inactive",
	// "HighTotal":    "high_total",
	// "LowTotal":     "low_total",
	// "LowFree":      "low_free",
	// "Dirty":        "dirty",
	// "Writeback":    "writeback",
	// "AnonPages":    "anon_pages",
	// "Mapped":       "mapped",
	// "Slab":         "slab",
	// "SReclaimable": "slab_reclaimable",
	// "SUnreclaim":   "slab_unreclaim",
	// "PageTables":   "page_tables",
	// "NFS_Unstable": "nfs_unstable",
	// "Bounce":       "bounce",
	// "CommitLimit":  "commit_limit",
	// "Committed_AS": "committed_as",
	// "VmallocTotal": "vmalloc_total",
	// "VmallocUsed":  "vmalloc_used",
	// "VmallocChunk": "vmalloc_chunk",
	// "SwapCached":   "swap_cached",
	// "SwapFree":     "swap_free",
}

func getMemoryInfo() (memoryInfo map[string]string, err error) {
	file, err := os.Open("/proc/meminfo")

	if err != nil {
		return
	}

	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if scanner.Err() != nil {
		err = scanner.Err()
		return
	}

	memoryInfo = make(map[string]string)

	for _, line := range lines {
		pair := regexp.MustCompile(": +").Split(line, 2)
		values := regexp.MustCompile(" +").Split(pair[1], 2)

		key, ok := memMap[pair[0]]
		if ok {
			memoryInfo[key] = fmt.Sprintf("%s%s", values[0], values[1])
		}
	}

	return
}
