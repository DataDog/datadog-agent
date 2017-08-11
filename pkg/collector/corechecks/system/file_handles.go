// +build !windows

package system

import (
	//"fmt"
	//"io"
	"io/ioutil"
	//"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	//"github.com/shirou/gopsutil/cpu"
	//"github.com/shirou/gopsutil/disk"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
//var times = cpu.Times
//var cpuInfo = cpu.Info
//var disk_test = disk.Partitions

// CPUCheck doesn't need additional fields
type fhCheck struct {
	nbCPU       float64
	lastNbCycle float64
	//lastTimes   cpu.TimesStat
}

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
	file_nr_handle := "/proc/sys/fs/file-nr"
	sender, err := aggregator.GetSender(c.ID())
	/*
		if err != nil {
			return err
		}
	*/
	//e_check(err)
	if sender == nil {
		return err
	}

	dat, err := ioutil.ReadFile(file_nr_handle)
	if err != nil {
		log.Errorf(err.Error())
	}

	file_nr_values := strings.Split(strings.TrimRight(string(dat), "\n"), "\t")

	allocated_fh, err := strconv.ParseFloat(file_nr_values[0], 64)
	//e_check(err)
	allocated_unused_fh, err := strconv.ParseFloat(file_nr_values[1], 64)
	//e_check(err)
	max_fh, err := strconv.ParseFloat(file_nr_values[2], 64)
	//e_check(err)

	fh_in_use := (allocated_fh - allocated_unused_fh) / max_fh
	/*
		log.Infof("lou allocated_fh type %s", reflect.TypeOf(allocated_fh).Kind())
		log.Infof("lou allocated_unused_fh type %s", reflect.TypeOf(allocated_unused_fh).Kind())
		log.Infof("lou max_fh type %s", reflect.TypeOf(max_fh).Kind())
		log.Infof("lou fh_in_use = %f", fh_in_use)
	*/
	sender.Gauge("system.fs.file_handles.in_use", fh_in_use, "", nil)
	sender.Commit()
	/*
		cpuTimes, err := times(false)
		if err != nil {
			log.Errorf("system.CPUCheck: could not retrieve cpu stats: %s", err)
			return err
		} else if len(cpuTimes) < 1 {
			errEmpty := fmt.Errorf("no cpu stats retrieve (empty results)")
			log.Errorf("system.CPUCheck: %s", errEmpty)
			return errEmpty
		}
		t := cpuTimes[0]

		nbCycle := t.Total() / c.nbCPU
	*/
	/*
		if c.lastNbCycle != 0 {
			// gopsutil return the sum of every CPU
			toPercent := 100 / (nbCycle - c.lastNbCycle)

			user := ((t.User + t.Nice) - (c.lastTimes.User + c.lastTimes.Nice)) / c.nbCPU
			system := ((t.System + t.Irq + t.Softirq) - (c.lastTimes.System + c.lastTimes.Irq + c.lastTimes.Softirq)) / c.nbCPU
			iowait := (t.Iowait - c.lastTimes.Iowait) / c.nbCPU
			idle := (t.Idle - c.lastTimes.Idle) / c.nbCPU
			stolen := (t.Stolen - c.lastTimes.Stolen) / c.nbCPU
			guest := (t.Guest - c.lastTimes.Guest) / c.nbCPU

				sender.Gauge("system.cpu.user", user*toPercent, "", nil)
				sender.Gauge("system.cpu.system", system*toPercent, "", nil)
				sender.Gauge("system.cpu.iowait", iowait*toPercent, "", nil)
				sender.Gauge("system.cpu.idle", idle*toPercent, "", nil)
				sender.Gauge("system.cpu.stolen", stolen*toPercent, "", nil)
				sender.Gauge("system.cpu.guest", guest*toPercent, "", nil)
				sender.Commit()

		}

		c.lastNbCycle = nbCycle
		c.lastTimes = t
	*/
	return nil
}

// Configure the CPU check doesn't need configuration
func (c *fhCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	/*
		info, err := cpuInfo()
		if err != nil {
			return fmt.Errorf("system.CPUCheck: could not query CPU info")
		}
		for _, i := range info {
			c.nbCPU += float64(i.Cores)
		}
	*/
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
