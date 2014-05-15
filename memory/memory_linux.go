package memory

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
)

var memMap = map[string]string{
	"MemTotal":  "total",
	"SwapTotal": "swap_total",
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
