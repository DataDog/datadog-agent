package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/cobra"
)

var (
	checkRate  int
	checkName  string
	checkDelay int
)

func init() {
	AgentCmd.AddCommand(checkCmd)

	checkCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to datadog.yaml")
	checkCmd.Flags().IntVarP(&checkRate, "check-rate", "r", 1, "check rate")
	checkCmd.Flags().StringVarP(&checkName, "check", "c", "", "check name")
	checkCmd.Flags().IntVarP(&checkDelay, "delay", "d", 100, "delay between running the check and grabbing the metrics in miliseconds")
	checkCmd.SetArgs([]string{"checkName"})
}

var checkCmd = &cobra.Command{
	Use:   "check <check_name>",
	Short: "Run the specified check",
	Long:  `Use this to run a specific check with a specific rate`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 0 {
			checkName = args[0]
		} else if checkName == "" {
			cmd.Help()
			os.Exit(0)
		}
		config.SetupLogger("off", "")

		common.SetupConfig(confFilePath)

		hostname, err := util.GetHostname()
		key := path.Join(util.AgentCachePrefix, "hostname")
		util.Cache.Set(key, hostname, util.NoExpiration)
		if err != nil {
			panic(err)
		}

		agg := aggregator.InitAggregator(common.Forwarder, hostname)
		agg.SetFlushInterval(10000000000)
		common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
		check := common.AC.GetCheck(checkName)
		if check == nil {
			fmt.Println("no check found")
			os.Exit(1)
		}
		i := 0
		for i < checkRate {
			err = check.Run()
			if err != nil {
				fmt.Printf("There was a problem running this check: %v\n", err)
				os.Exit(1)
			}
			i++
		}

		// Without a small delay some of the metrics will not show up
		time.Sleep(100 * time.Millisecond)

		series := agg.GetSeries()
		if len(series) != 0 {
			fmt.Println("Series: ")
			for _, s := range series {
				j, _ := json.Marshal(s)
				fmt.Println(string(j))
			}
		}

		sketches := agg.GetSketches()
		if len(sketches) != 0 {
			fmt.Println("Sketches: ")
			for _, s := range sketches {
				j, _ := json.Marshal(s)
				fmt.Println(string(j))
			}
		}
		serviceChecks := agg.GetServiceChecks()
		if len(serviceChecks) != 0 {
			fmt.Println("Service Checks: ")
			for _, s := range serviceChecks {
				j, _ := json.Marshal(s)
				fmt.Println(string(j))
			}
		}
		events := agg.GetEvents()
		if len(events) != 0 {
			fmt.Println("Events: ")
			for _, e := range events {
				j, _ := json.Marshal(e)
				fmt.Println(string(j))
			}
		}

		metricsJSON, _ := agg.GetMetrics(check.ID())
		metrics := []*aggregator.Serie{}
		json.Unmarshal(metricsJSON, &metrics)
		if len(metrics) != 0 {
			fmt.Println("Metrics: ")
			for _, m := range metrics {
				j, _ := json.Marshal(m)
				fmt.Println(string(j))
			}
		}

	},
}
