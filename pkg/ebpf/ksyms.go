package ebpf

import (
	"bufio"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// VerifyKernelFuncs ensures all kernel functions exist in ksyms located at provided path.
func VerifyKernelFuncs(path string, requiredKernelFuncs []string) ([]string, error) {
	// Will hold the found functions
	found := make(map[string]bool, len(requiredKernelFuncs))
	for _, f := range requiredKernelFuncs {
		found[f] = false
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading kallsyms file from: %s", path)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		name := fields[2]
		if _, ok := found[name]; ok {
			found[name] = true
		}
	}

	var missing []string
	for probe, b := range found {
		if !b {
			missing = append(missing, probe)
		}
	}

	return missing, nil
}
