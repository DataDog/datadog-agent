package helpers

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/fatih/color"
	"github.com/hashicorp/go-multierror"
)

// ReadProfileData collects profiles from the agent and returns them in a ProfileData object.
// Currently sequential execution, althought some of it could be parallelized.
func ReadProfileData(seconds int) (types.ProfileData, error) {
	type agentProfileCollector func(service string) error

	pdata := types.ProfileData{}
	c := util.GetClient(false)

	type pprofGetter func(path string) ([]byte, error)

	tcpGet := func(portConfig string) pprofGetter {
		pprofURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", pkgconfig.Datadog.GetInt(portConfig))
		return func(path string) ([]byte, error) {
			return util.DoGet(c, pprofURL+path, util.LeaveConnectionOpen)
		}
	}

	serviceProfileCollector := func(get func(url string) ([]byte, error), seconds int) agentProfileCollector {
		return func(service string) error {
			fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from %s.", seconds, service))
			for _, prof := range []struct{ name, path string }{
				{
					// 1st heap profile
					name: service + "-1st-heap.pprof",
					path: "/heap",
				},
				{
					// CPU profile
					name: service + "-cpu.pprof",
					path: fmt.Sprintf("/profile?seconds=%d", seconds),
				},
				{
					// 2nd heap profile
					name: service + "-2nd-heap.pprof",
					path: "/heap",
				},
				{
					// mutex profile
					name: service + "-mutex.pprof",
					path: "/mutex",
				},
				{
					// goroutine blocking profile
					name: service + "-block.pprof",
					path: "/block",
				},
			} {
				b, err := get(prof.path)
				if err != nil {
					return err
				}
				pdata[prof.name] = b
			}
			return nil
		}
	}

	agentCollectors := map[string]agentProfileCollector{
		"core":           serviceProfileCollector(tcpGet("expvar_port"), seconds),
		"security-agent": serviceProfileCollector(tcpGet("security_agent.expvar_port"), seconds),
	}

	if pkgconfig.Datadog.GetBool("process_config.enabled") ||
		pkgconfig.Datadog.GetBool("process_config.container_collection.enabled") ||
		pkgconfig.Datadog.GetBool("process_config.process_collection.enabled") {

		agentCollectors["process"] = serviceProfileCollector(tcpGet("process_config.expvar_port"), seconds)
	}

	if pkgconfig.Datadog.GetBool("apm_config.enabled") {
		traceCpusec := pkgconfig.Datadog.GetInt("apm_config.receiver_timeout")
		if traceCpusec > seconds {
			// do not exceed requested duration
			traceCpusec = seconds
		} else if traceCpusec <= 0 {
			// default to 4s as maximum connection timeout of trace-agent HTTP server is 5s by default
			traceCpusec = 4
		}

		agentCollectors["trace"] = serviceProfileCollector(tcpGet("apm_config.debug.port"), traceCpusec)
	}

	if pkgconfig.SystemProbe.GetBool("system_probe_config.enabled") {
		probeUtil, probeUtilErr := net.GetRemoteSystemProbeUtil(pkgconfig.SystemProbe.GetString("system_probe_config.sysprobe_socket"))

		if !errors.Is(probeUtilErr, net.ErrNotImplemented) {
			sysProbeGet := func() pprofGetter {
				return func(path string) ([]byte, error) {
					if probeUtilErr != nil {
						return nil, probeUtilErr
					}

					return probeUtil.GetPprof(path)
				}
			}

			agentCollectors["system-probe"] = serviceProfileCollector(sysProbeGet(), seconds)
		}
	}

	var errs error
	for name, callback := range agentCollectors {
		if err := callback(name); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("error collecting %s agent profile: %v", name, err))
		}
	}

	return pdata, errs
}

// RequestInternalProfiling requests to start, sleep for N seconds
// and stop internal Datadog profiling for the Agent processes
// Execution is parallelized.
func RequestInternalProfiling(seconds int) error {

	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return err
	}

	c := util.GetClient(false)

	var errs error
	var sysProbeErr error
	var sysProbe *net.RemoteSysProbeUtil
	cmdPort := pkgconfig.Datadog.GetInt("cmd_port")

	// Request start of internal profiling for core
	url := fmt.Sprintf("https://%v:%v/agent/internal-profile/start", ipcAddress, cmdPort)
	_, coreErr := util.DoGet(c, url, util.LeaveConnectionOpen)
	if coreErr != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to start collecting core agent internal profiling: %v", coreErr))
	}

	// Request to start of internal profiling for system probe
	if pkgconfig.SystemProbe.GetBool("system_probe_config.enabled") {
		sysProbe, sysProbeErr = net.GetRemoteSystemProbeUtil(pkgconfig.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
		if sysProbeErr == nil {
			sysProbeErr := sysProbe.RequestInternalProfiling(true)
			if sysProbeErr != nil {
				errs = multierror.Append(errs, fmt.Errorf("failed to start collecting system probe internal profiling: %v", sysProbeErr))
			}
		} else {
			errs = multierror.Append(errs, fmt.Errorf("failed to start collecting system probe internal profiling: %v", sysProbeErr))
		}
	}

	// Wait for configured seconds
	time.Sleep(time.Duration(seconds) * time.Second)

	// Stop internal profiling for core
	if coreErr == nil {
		url := fmt.Sprintf("https://%v:%v/agent/internal-profile/stop", ipcAddress, cmdPort)
		_, coreErr := util.DoGet(c, url, util.LeaveConnectionOpen)
		if coreErr != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to stop collecting core agent internal profiling: %v", coreErr))
		}
	}

	// Stop internal profiling for system probe
	if sysProbeErr == nil {
		sysProbeErr := sysProbe.RequestInternalProfiling(true)
		if sysProbeErr != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to stop collecting system probe internal profiling: %v", sysProbeErr))
		}
	}

	//	return errs
	return nil
}
