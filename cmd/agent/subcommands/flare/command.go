// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements 'agent flare'.
package flare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/streamlogs"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager/diagnosesendermanagerimpl"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryotel/inventoryotelimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	procnet "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args are the positional command-line arguments
	args []string

	// subcommand-specific flags

	customerEmail        string
	autoconfirm          bool
	forceLocal           bool
	profiling            int
	profileMutex         bool
	profileMutexFraction int
	profileBlocking      bool
	profileBlockingRate  int
	withStreamLogs       time.Duration
	providerTimeout      time.Duration
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	flareCmd := &cobra.Command{
		Use:   "flare [caseID]",
		Short: "Collect a flare and send it to Datadog",
		Long:  ``,
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.args = args
			config := config.NewAgentParams(globalParams.ConfFilePath,
				config.WithSecurityAgentConfigFilePaths([]string{
					path.Join(defaultpaths.ConfPath, "security-agent.yaml"),
				}),
				config.WithConfigLoadSecurityAgent(true),
				config.WithIgnoreErrors(true),
				config.WithExtraConfFiles(globalParams.ExtraConfFilePath),
				config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
			)

			return fxutil.OneShot(makeFlare,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams:         config,
					SecretParams:         secrets.NewEnabledParams(),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:            log.ForOneShot(command.LoggerName, "off", false),
				}),
				flare.Module(flare.NewLocalParams(
					defaultpaths.GetDistPath(),
					defaultpaths.PyChecksPath,
					defaultpaths.LogFile,
					defaultpaths.JmxLogFile,
					defaultpaths.DogstatsDLogFile,
					defaultpaths.StreamlogsLogFile,
				)),
				// workloadmeta setup
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(workloadmeta.Params{
					AgentType:  workloadmeta.NodeAgent,
					InitHelper: common.GetWorkloadmetaInit(),
				}),
				fx.Provide(tagger.NewTaggerParams),
				taggerimpl.Module(),
				autodiscoveryimpl.Module(),
				fx.Supply(optional.NewNoneOption[collector.Component]()),
				compressionimpl.Module(),
				diagnosesendermanagerimpl.Module(),
				// We need inventoryagent to fill the status page generated by the flare.
				inventoryagentimpl.Module(),
				hostimpl.Module(),
				inventoryhostimpl.Module(),
				inventoryotelimpl.Module(),
				resourcesimpl.Module(),
				authtokenimpl.Module(),
				// inventoryagent require a serializer. Since we're not actually sending the payload to
				// the backend a nil will work.
				fx.Provide(func() serializer.MetricSerializer {
					return nil
				}),
				core.Bundle(),
			)
		},
	}

	flareCmd.Flags().StringVarP(&cliParams.customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&cliParams.autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().BoolVarP(&cliParams.forceLocal, "local", "l", false, "Force the creation of the flare by the command line instead of the agent process (useful when running in a containerized env)")
	flareCmd.Flags().IntVarP(&cliParams.profiling, "profile", "p", -1, "Add performance profiling data to the flare. It will collect a heap profile and a CPU profile for the amount of seconds passed to the flag, with a minimum of 30s")
	flareCmd.Flags().BoolVarP(&cliParams.profileMutex, "profile-mutex", "M", false, "Add mutex profile to the performance data in the flare")
	flareCmd.Flags().IntVarP(&cliParams.profileMutexFraction, "profile-mutex-fraction", "", 100, "Set the fraction of mutex contention events that are reported in the mutex profile")
	flareCmd.Flags().BoolVarP(&cliParams.profileBlocking, "profile-blocking", "B", false, "Add gorouting blocking profile to the performance data in the flare")
	flareCmd.Flags().IntVarP(&cliParams.profileBlockingRate, "profile-blocking-rate", "", 10000, "Set the fraction of goroutine blocking events that are reported in the blocking profile")
	flareCmd.Flags().DurationVarP(&cliParams.withStreamLogs, "with-stream-logs", "L", 0*time.Second, "Add stream-logs data to the flare. It will collect logs for the amount of seconds passed to the flag")
	flareCmd.Flags().DurationVarP(&cliParams.providerTimeout, "provider-timeout", "t", 0*time.Second, "Timeout to run each flare provider in seconds. This is not a global timeout for the flare creation process.")
	flareCmd.SetArgs([]string{"caseID"})

	return []*cobra.Command{flareCmd}
}

func readProfileData(seconds int) (flare.ProfileData, error) {
	type agentProfileCollector func(service string) error

	pdata := flare.ProfileData{}
	c := util.GetClient(false)

	type pprofGetter func(path string) ([]byte, error)

	tcpGet := func(portConfig string) pprofGetter {
		pprofURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", pkgconfigsetup.Datadog().GetInt(portConfig))
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
				{
					// Trace
					name: service + ".trace",
					path: fmt.Sprintf("/trace?seconds=%d", seconds),
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

	if pkgconfigsetup.Datadog().GetBool("process_config.enabled") ||
		pkgconfigsetup.Datadog().GetBool("process_config.container_collection.enabled") ||
		pkgconfigsetup.Datadog().GetBool("process_config.process_collection.enabled") {

		agentCollectors["process"] = serviceProfileCollector(tcpGet("process_config.expvar_port"), seconds)
	}

	if pkgconfigsetup.Datadog().GetBool("apm_config.enabled") {
		traceCpusec := pkgconfigsetup.Datadog().GetInt("apm_config.receiver_timeout")
		if traceCpusec > seconds {
			// do not exceed requested duration
			traceCpusec = seconds
		} else if traceCpusec <= 0 {
			// default to 4s as maximum connection timeout of trace-agent HTTP server is 5s by default
			traceCpusec = 4
		}

		agentCollectors["trace"] = serviceProfileCollector(tcpGet("apm_config.debug.port"), traceCpusec)
	}

	if pkgconfigsetup.SystemProbe().GetBool("system_probe_config.enabled") {
		probeUtil, probeUtilErr := procnet.GetRemoteSystemProbeUtil(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"))

		if !errors.Is(probeUtilErr, procnet.ErrNotImplemented) {
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

func makeFlare(flareComp flare.Component,
	lc log.Component,
	config config.Component,
	_ sysprobeconfig.Component,
	cliParams *cliParams,
	_ optional.Option[workloadmeta.Component],
	_ tagger.Component) error {
	var (
		profile flare.ProfileData
		err     error
	)

	streamLogParams := streamlogs.CliParams{
		FilePath: defaultpaths.StreamlogsLogFile,
		Duration: cliParams.withStreamLogs,
		Quiet:    true,
	}

	if streamLogParams.Duration < 0 {
		fmt.Fprintln(color.Output, color.YellowString("Invalid duration provided for streaming logs, please provide a positive value"))
	}

	fmt.Fprintln(color.Output, color.BlueString("NEW: You can now generate a flare from the comfort of your Datadog UI!"))
	fmt.Fprintln(color.Output, color.BlueString("See https://docs.datadoghq.com/agent/troubleshooting/send_a_flare/?tab=agentv6v7#send-a-flare-from-the-datadog-site for more info."))

	warnings := config.Warnings()
	if warnings != nil && warnings.Err != nil {
		fmt.Fprintln(color.Error, color.YellowString("Config parsing warning: %v", warnings.Err))
	}
	caseID := ""
	if len(cliParams.args) > 0 {
		caseID = cliParams.args[0]
	}

	customerEmail := cliParams.customerEmail
	if customerEmail == "" {
		customerEmail, err = input.AskForEmail()
		if err != nil {
			fmt.Println("Error reading email, please retry or contact support")
			return err
		}
	}

	if cliParams.profiling >= 30 {
		c, err := common.NewSettingsClient()
		if err != nil {
			return fmt.Errorf("failed to initialize settings client: %w", err)
		}

		profilingOpts := settings.ProfilingOpts{
			ProfileMutex:         cliParams.profileMutex,
			ProfileMutexFraction: cliParams.profileMutexFraction,
			ProfileBlocking:      cliParams.profileBlocking,
			ProfileBlockingRate:  cliParams.profileBlockingRate,
		}

		err = settings.ExecWithRuntimeProfilingSettings(func() {
			if profile, err = readProfileData(cliParams.profiling); err != nil {
				fmt.Fprintln(color.Output, color.YellowString(fmt.Sprintf("Could not collect performance profile data: %s", err)))
			}
		}, profilingOpts, c)
		if err != nil {
			return err
		}
	} else if cliParams.profiling != -1 {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Invalid value for profiling: %d. Please enter an integer of at least 30.", cliParams.profiling)))
		return err
	}

	if streamLogParams.Duration > 0 {
		fmt.Fprintln(color.Output, color.GreenString((fmt.Sprintf("Asking the agent to stream logs for %s", streamLogParams.Duration))))
		err := streamlogs.StreamLogs(lc, config, &streamLogParams)
		if err != nil {
			fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error streaming logs: %s", err)))
		}
	}

	var filePath string
	if cliParams.forceLocal {
		filePath, err = createArchive(flareComp, profile, cliParams.providerTimeout, nil)
	} else {
		filePath, err = requestArchive(flareComp, profile, cliParams.providerTimeout)
	}

	if err != nil {
		return err
	}

	if _, err := os.Stat(filePath); err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("The flare zipfile \"%s\" does not exist.", filePath)))
		fmt.Fprintln(color.Output, color.RedString("If the agent running in a different container try the '--local' option to generate the flare locally"))
		return err
	}

	fmt.Fprintf(color.Output, "%s is going to be uploaded to Datadog\n", color.YellowString(filePath))
	if !cliParams.autoconfirm {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintf(color.Output, "Aborting. (You can still use %s)\n", color.YellowString(filePath))
			return nil
		}
	}

	response, e := flareComp.Send(filePath, caseID, customerEmail, helpers.NewLocalFlareSource())
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}

func requestArchive(flareComp flare.Component, pdata flare.ProfileData, providerTimeout time.Duration) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(flareComp, pdata, providerTimeout, err)
	}

	cmdport := pkgconfigsetup.Datadog().GetInt("cmd_port")
	url := &url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(ipcAddress, strconv.Itoa(cmdport)),
		Path:   "/agent/flare",
	}
	if providerTimeout > 0 {
		q := url.Query()
		q.Set("provider_timeout", strconv.FormatInt(int64(providerTimeout), 10))
		url.RawQuery = q.Encode()
	}

	urlstr := url.String()

	// Set session token
	if err = util.SetAuthToken(pkgconfigsetup.Datadog()); err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", err)))
		return createArchive(flareComp, pdata, providerTimeout, err)
	}

	p, err := json.Marshal(pdata)
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error while encoding profile: %s", err)))
		return "", err
	}

	r, err := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer(p))
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintf(color.Output, "The agent ran into an error while making the flare: %s\n", color.RedString(string(r)))
			err = fmt.Errorf("Error getting flare from running agent: %s", r)
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
			err = fmt.Errorf("Error getting flare from running agent: %w", err)
		}
		return createArchive(flareComp, pdata, providerTimeout, err)
	}

	return string(r), nil
}

func createArchive(flareComp flare.Component, pdata flare.ProfileData, providerTimeout time.Duration, ipcError error) (string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	filePath, err := flareComp.Create(pdata, providerTimeout, ipcError)
	if err != nil {
		fmt.Printf("The flare zipfile failed to be created: %s\n", err)
		return "", err
	}

	return filePath, nil
}
