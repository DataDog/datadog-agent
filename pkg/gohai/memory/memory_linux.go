package verity

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
)

type Memory struct{}

func (self *Memory) Collect() (result map[string]map[string]string, err error) {
	memoryInfo, err := getMemoryInfo()
	result = map[string]map[string]string{
		"memory": memoryInfo,
	}

	return
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

		switch pair[0] {
		case "MemTotal":
			memoryInfo["total"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "MemFree":
			memoryInfo["free"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Buffers":
			memoryInfo["buffers"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Cached":
			memoryInfo["cached"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Active":
			memoryInfo["active"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Inactive":
			memoryInfo["inactive"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "HighTotal":
			memoryInfo["high_total"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "HighFree":
			memoryInfo["high_free"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "LowTotal":
			memoryInfo["low_total"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "LowFree":
			memoryInfo["low_free"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Dirty":
			memoryInfo["dirty"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Writeback":
			memoryInfo["writeback"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "AnonPages":
			memoryInfo["anon_pages"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Mapped":
			memoryInfo["mapped"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Slab":
			memoryInfo["slab"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "SReclaimable":
			memoryInfo["slab_reclaimable"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "SUnreclaim":
			memoryInfo["slab_unreclaim"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "PageTables":
			memoryInfo["page_tables"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "NFS_Unstable":
			memoryInfo["nfs_unstable"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Bounce":
			memoryInfo["bounce"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "CommitLimit":
			memoryInfo["commit_limit"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "Committed_AS":
			memoryInfo["committed_as"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "VmallocTotal":
			memoryInfo["vmalloc_total"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "VmallocUsed":
			memoryInfo["vmalloc_used"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "VmallocChunk":
			memoryInfo["vmalloc_chunk"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "SwapCached":
			memoryInfo["swap_cached"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "SwapTotal":
			memoryInfo["swap_total"] = fmt.Sprintf("%s%s", values[0], values[1])
		case "SwapFree":
			memoryInfo["swap_free"] = fmt.Sprintf("%s%s", values[0], values[1])
		}
	}

	return
}
