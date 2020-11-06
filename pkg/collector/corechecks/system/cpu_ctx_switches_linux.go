// +build linux

package system

func readCtxSwitches(procStatPath string) (ctxSwitches int64, err error) {
	file, err := os.Open(procStatPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		txt := scanner.Text()
		if strings.HasPrefix(txt, "ctxt") {
			elemts := strings.Split(txt, " ")
			ctxSwitches, err = strconv.ParseInt(elemts[1], 10, 64)
			if err != nil {
				return 0, err
			}
			return ctxSwitches, nil
		}
	}

	return 0, fmt.Errorf("could not find the context switches in stat file")
}

func (c *CPUCheck) collectCtxSwitches() error {
	// TODO: Make me configurable
	ctxSwitches, err := readCtxSwitches("/proc/stat")
	if err != nil {
		log.Warnf("system.CPUCheck could not read context switches: %s", err.Error())
		return err
	}
	sender.MonotonicCount("system.linux.context_switches", float64(ctxSwitches), "", nil)
	return nil
}
