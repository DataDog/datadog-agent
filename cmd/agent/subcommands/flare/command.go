// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements 'agent flare'.
package flare

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/streamlogs"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager/diagnosesendermanagerimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/diagnose/format"
	diagnosefx "github.com/DataDog/datadog-agent/comp/core/diagnose/fx"
	diagnoseLocal "github.com/DataDog/datadog-agent/comp/core/diagnose/local"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	flareprofilerdef "github.com/DataDog/datadog-agent/comp/core/profiler/def"
	flareprofilerfx "github.com/DataDog/datadog-agent/comp/core/profiler/fx"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	coresettings "github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	localTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
	workloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	haagentmetadatafx "github.com/DataDog/datadog-agent/comp/metadata/haagent/fx"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryotel/inventoryotelimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	logscompressorfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompressorfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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
	logLevelDefaultOff   command.LogLevelDefaultOff
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
			c := config.NewAgentParams(globalParams.ConfFilePath,
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
					ConfigParams:         c,
					SecretParams:         secrets.NewEnabledParams(),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:            log.ForOneShot(command.LoggerName, cliParams.logLevelDefaultOff.Value(), false),
				}),
				flare.Module(flare.NewLocalParams(
					defaultpaths.GetDistPath(),
					defaultpaths.PyChecksPath,
					defaultpaths.LogFile,
					defaultpaths.JmxLogFile,
					defaultpaths.DogstatsDLogFile,
					defaultpaths.StreamlogsLogFile,
				)),
				flareprofilerfx.Module(),
				// workloadmeta setup
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(workloadmeta.Params{
					AgentType:  workloadmeta.NodeAgent,
					InitHelper: common.GetWorkloadmetaInit(),
				}),
				fx.Provide(func(config config.Component) coresettings.Params {
					return coresettings.Params{
						// A settings object is required to populate some dependencies, but
						// no values are valid since the flare runs by default in a separate
						// process from the main agent.
						Settings: map[string]coresettings.RuntimeSetting{},
						Config:   config,
					}
				}),
				settingsimpl.Module(),
				localTaggerfx.Module(),
				workloadfilterfx.Module(),
				autodiscoveryimpl.Module(),
				fx.Supply(option.None[collector.Component]()),
				diagnosesendermanagerimpl.Module(),
				// We need inventoryagent to fill the status page generated by the flare.
				inventoryagentimpl.Module(),
				hostimpl.Module(),
				inventoryhostimpl.Module(),
				inventoryotelimpl.Module(),
				haagentmetadatafx.Module(),
				resourcesimpl.Module(),
				// inventoryagent require a serializer. Since we're not actually sending the payload to
				// the backend a nil will work.
				fx.Provide(func() serializer.MetricSerializer {
					return nil
				}),
				core.Bundle(),
				haagentfx.Module(),
				logscompressorfx.Module(),
				metricscompressorfx.Module(),
				diagnosefx.Module(),
				ipcfx.ModuleInsecure(),
			)
		},
	}
	cliParams.logLevelDefaultOff.Register(flareCmd)

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

func makeFlare(flareComp flare.Component,
	lc log.Component,
	config config.Component,
	_ sysprobeconfig.Component,
	cliParams *cliParams,
	_ option.Option[workloadmeta.Component],
	tagger tagger.Component,
	flareprofiler flareprofilerdef.Component,
	client ipc.HTTPClient,
	senderManager diagnosesendermanager.Component,
	wmeta option.Option[workloadmeta.Component],
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	diagnoseComponent diagnose.Component,
) error {
	var (
		profile flaretypes.ProfileData
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
	if warnings != nil && warnings.Errors != nil {
		fmt.Fprintln(color.Error, color.YellowString("Config parsing warning: %v", warnings.Errors))
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
		c, err := common.NewSettingsClient(client)
		if err != nil {
			return fmt.Errorf("failed to initialize settings client: %w", err)
		}

		profilingOpts := settings.ProfilingOpts{
			ProfileMutex:         cliParams.profileMutex,
			ProfileMutexFraction: cliParams.profileMutexFraction,
			ProfileBlocking:      cliParams.profileBlocking,
			ProfileBlockingRate:  cliParams.profileBlockingRate,
		}

		logFunc := func(s string, params ...interface{}) error {
			fmt.Fprintln(color.Output, color.BlueString(s, params...))
			return nil
		}

		err = settings.ExecWithRuntimeProfilingSettings(func() {
			if profile, err = flareprofiler.ReadProfileData(cliParams.profiling, logFunc); err != nil {
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
		err := streamlogs.StreamLogs(lc, config, client, &streamLogParams)
		if err != nil {
			fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error streaming logs: %s", err)))
		}
	}

	var filePath string

	if cliParams.forceLocal {
		diagnoseresult := runLocalDiagnose(diagnoseComponent, diagnose.Config{Verbose: true}, lc, senderManager, wmeta, ac, secretResolver, tagger, config)
		filePath, err = createArchive(flareComp, profile, cliParams.providerTimeout, nil, diagnoseresult)
	} else {
		filePath, err = requestArchive(profile, client, cliParams.providerTimeout)
		if err != nil {
			diagnoseresult := runLocalDiagnose(diagnoseComponent, diagnose.Config{Verbose: true}, lc, senderManager, wmeta, ac, secretResolver, tagger, config)
			filePath, err = createArchive(flareComp, profile, cliParams.providerTimeout, err, diagnoseresult)
		}
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

func requestArchive(pdata flaretypes.ProfileData, client ipc.HTTPClient, providerTimeout time.Duration) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return "", err
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

	p, err := json.Marshal(pdata)
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error while encoding profile: %s", err)))
		return "", err
	}

	r, err := client.Post(urlstr, "application/json", bytes.NewBuffer(p))
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintf(color.Output, "The agent ran into an error while making the flare: %s\n", color.RedString(string(r)))
			err = fmt.Errorf("Error getting flare from running agent: %s", r)
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
			err = fmt.Errorf("Error getting flare from running agent: %w", err)
		}
		return "", err
	}

	return string(r), nil
}

func createArchive(flareComp flare.Component, pdata flaretypes.ProfileData, providerTimeout time.Duration, ipcError error, diagnoseResult []byte) (string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	filePath, err := flareComp.Create(pdata, providerTimeout, ipcError, diagnoseResult)
	if err != nil {
		fmt.Printf("The flare zipfile failed to be created: %s\n", err)
		return "", err
	}

	return filePath, nil
}

func runLocalDiagnose(
	diagnoseComponent diagnose.Component,
	diagnoseConfig diagnose.Config,
	log log.Component,
	senderManager diagnosesendermanager.Component,
	wmeta option.Option[workloadmeta.Component],
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	tagger tagger.Component,
	config config.Component) []byte {

	result, err := diagnoseLocal.Run(diagnoseComponent, diagnose.Config{Verbose: true}, log, senderManager, wmeta, ac, secretResolver, tagger, config)

	if err != nil {
		return []byte(color.RedString(fmt.Sprintf("Error running diagnose: %s", err)))
	}

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	err = format.Text(writer, diagnoseConfig, result)
	if err != nil {
		return []byte(color.RedString(fmt.Sprintf("Error formatting diagnose result: %s", err)))
	}
	writer.Flush()

	return buffer.Bytes()
}
