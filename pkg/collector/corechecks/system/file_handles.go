// +build !windows

package system

import (
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type fhCheck struct{}

func (c *fhCheck) String() string {
	return "disk"
}

func e_check(e error) error {
	if e != nil {
		return e
	}
	return e
}

// Run executes the check
func (c *fhCheck) Run() error {
	// https://www.kernel.org/doc/Documentation/sysctl/fs.txt
	file_nr_handle := "/proc/sys/fs/file-nr"

	sender, err := aggregator.GetSender(c.ID())
	e_check(err)
	if sender == nil {
		return err
	}

	dat, err := ioutil.ReadFile(file_nr_handle)
	if err != nil {
		log.Errorf(err.Error())
	}

	file_nr_values := strings.Split(strings.TrimRight(string(dat), "\n"), "\t")

	allocated_fh, err := strconv.ParseFloat(file_nr_values[0], 64)
	e_check(err)
	log.Debugf("Allocated File Handles: %f", allocated_fh)
	allocated_unused_fh, err := strconv.ParseFloat(file_nr_values[1], 64)
	e_check(err)
	log.Debugf("Allocated Unused File Handles: %f", allocated_unused_fh)
	max_fh, err := strconv.ParseFloat(file_nr_values[2], 64)
	e_check(err)

	fh_in_use := (allocated_fh - allocated_unused_fh) / max_fh
	log.Debugf("File Handles In Use: %f", fh_in_use)

	sender.Gauge("system.fs.file_handles.in_use", fh_in_use, "", nil)
	sender.Commit()

	return nil
}

// The check doesn't need configuration
func (c *fhCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	return nil
}

// Interval returns the scheduling time for the check
func (c *fhCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *fhCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *fhCheck) Stop() {}

func fhFactory() check.Check {
	return &fhCheck{}
}

func init() {
	core.RegisterCheck("disk", fhFactory)
}
