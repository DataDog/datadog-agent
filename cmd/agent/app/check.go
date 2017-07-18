package app

import (
	"path"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/spf13/cobra"

	log "github.com/cihub/seelog"
)

var (
	checkRate int
	checkName string
)

func init() {
	AgentCmd.AddCommand(checkCmd)

	checkCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to datadog.yaml")
	checkCmd.Flags().IntVarP(&checkRate, "check-rate", "r", 0, "check rate")
	checkCmd.Flags().StringVarP(&checkName, "check", "c", "", "check name")
	checkCmd.SetArgs([]string{"checkName"})
}

var checkCmd = &cobra.Command{
	Use:   "check <check_name>",
	Short: "Run the specified check",
	Long:  `Use this to run a specific check with a specific rate`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 0 {
			checkName = args[0]
		}

		common.SetupConfig(confFilePath)

		hostname, err := util.GetHostname()
		key := path.Join(util.AgentCachePrefix, "hostname")
		util.Cache.Set(key, hostname, util.NoExpiration)
		if err != nil {
			panic(err)
		}

		keysPerDomain, err := config.GetMultipleEndpoints()
		common.Forwarder = forwarder.NewDefaultForwarder(keysPerDomain)
		log.Debugf("Starting forwarder")
		common.Forwarder.Start()
		log.Debugf("Forwarder started")

		agg := aggregator.InitAggregator(common.Forwarder, hostname)
		agg.AddAgentStartupEvent(version.AgentVersion)

		common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
		common.AC.RunCheck(checkName)
	},
}
