// +build linux_bpf

package ebpf

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

type KprobeStats struct {
	Hits   int64
	Misses int64
}

// KprobeProfile is the default path to the kprobe_profile file
const KprobeProfile = "/sys/kernel/debug/tracing/kprobe_profile"

// GetProbeStats gathers stats about the # of kprobes triggered /missed by reading the kprobe_profile file
func GetProbeStats() map[string]int64 {
	m, err := readKprobeProfile(KprobeProfile)
	if err != nil {
		log.Debugf("error retrieving probe stats: %s", err)
		return map[string]int64{}
	}

	res := make(map[string]int64, 2*len(m))
	for event, st := range m {
		res[fmt.Sprintf("%s_hits", event)] = st.Hits
		res[fmt.Sprintf("%s_misses", event)] = st.Misses
	}

	return res
}

// GetProbeTotals returns the total number of kprobes triggered or missed by reading the kprobe_profile file
func GetProbeTotals() KprobeStats {
	stats := KprobeStats{}
	m, err := readKprobeProfile(KprobeProfile)
	if err != nil {
		log.Debugf("error retrieving probe stats: %s", err)
		return stats
	}

	for _, st := range m {
		stats.Hits += st.Hits
		stats.Misses += st.Misses
	}
	return stats
}

// readKprobeProfile reads a /sys/kernel/debug/tracing/kprobe_profile file and returns a map of probe -> stats
func readKprobeProfile(path string) (map[string]KprobeStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening kprobe profile file at: %s", path)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	stats := map[string]KprobeStats{}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}

		hits, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			log.Debugf("error parsing kprobe_profile output for hits (%s): %s", fields[1], err)
			continue
		}

		misses, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			log.Debugf("error parsing kprobe_profile output for miss (%s): %s", fields[2], err)
			continue
		}

		stats[fields[0]] = KprobeStats{
			Hits:   hits,
			Misses: misses,
		}
	}

	return stats, nil
}
