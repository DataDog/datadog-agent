package app

import (
	"encoding/json"
	"expvar"
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
		// config.SetupLogger("off", "")

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
		err = check.Run()
		if err != nil {
			fmt.Printf("There was a problem running this check: %v\n", err)
			os.Exit(1)
		}
		err = check.Run()
		if err != nil {
			fmt.Printf("There was a problem running this check: %v\n", err)
			os.Exit(1)
		}

		time.Sleep(15)

		series := agg.GetSeries()
		fmt.Println("Series: ")
		fmt.Println(series)
		if len(series) != 0 {
			fmt.Println("Series: ")
			for s := range series {
				fmt.Println(json.Marshal(s))
			}
		}

		sketches := agg.GetSketches()
		fmt.Println("Sketches: ")
		fmt.Println(sketches)
		if len(sketches) != 0 {
			fmt.Println("Sketches: ")
			for s := range sketches {
				fmt.Println(json.Marshal(s))
			}
		}
		serviceChecks := agg.GetServiceChecks()
		fmt.Println("Service Checks: ")
		fmt.Println(serviceChecks)
		if len(serviceChecks) != 0 {
			fmt.Println("Service Checks: ")
			for s := range serviceChecks {
				fmt.Println(json.Marshal(s))
			}
		}
		events := agg.GetEvents()
		fmt.Println("Events: ")
		fmt.Println(events)
		if len(events) != 0 {
			fmt.Println("Events: ")
			for e := range events {
				fmt.Println(json.Marshal(e))
			}
		}

		metrics, _ := agg.GetMetrics(check.ID())

		fmt.Println(string(metrics))

		aggregatorStatsJSON := []byte(expvar.Get("aggregator").String())
		fmt.Println(string(aggregatorStatsJSON))

	},
}
