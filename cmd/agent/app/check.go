package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/cobra"
)

var (
	checkRate  bool
	checkName  string
	checkDelay int
)

func init() {
	AgentCmd.AddCommand(checkCmd)

	checkCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to datadog.yaml")
	checkCmd.Flags().BoolVarP(&checkRate, "check-rate", "r", false, "check rates by running the check twice")
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
		c := common.AC.GetCheck(checkName)
		if c == nil {
			fmt.Println("no check found")
			os.Exit(1)
		}

		s := check.NewStats(c)
		i := 0
		times := 1
		if checkRate {
			times = 2
		}
		for i < times {
			t0 := time.Now()
			err = c.Run()
			s.Add(time.Since(t0), err)
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

		checkStatus, _ := status.GetCheckStatus(c, s)
		fmt.Println(string(checkStatus))

	},
}
