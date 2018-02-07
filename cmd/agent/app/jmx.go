package app

import (
	"fmt"
	"log"

	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"github.com/spf13/cobra"
)

var (
	jmxCmd = &cobra.Command{
		Use:   "jmx",
		Short: "",
		Long:  ``,
	}

	jmxListCmd = &cobra.Command{
		Use:   "list",
		Short: "List attributes matched by JMXFetch.",
		Long:  ``,
	}

	jmxListEverythingCmd = &cobra.Command{
		Use:   "everything",
		Short: "List every attributes available that has a type supported by JMXFetch.",
		Long:  ``,
		Run:   doJmxListEverything,
	}

	jmxListMatchingCmd = &cobra.Command{
		Use:   "matching",
		Short: "List attributes that match at least one of your instances configuration.",
		Long:  ``,
		Run:   doJmxListMatching,
	}

	jmxListLimitedCmd = &cobra.Command{
		Use:   "limited",
		Short: "List attributes that do match one of your instances configuration but that are not being collected because it would exceed the number of metrics that can be collected.",
		Long:  ``,
		Run:   doJmxListLimited,
	}

	jmxListCollectedCmd = &cobra.Command{
		Use:   "collected",
		Short: "List attributes that will actually be collected by your current instances configuration.",
		Long:  ``,
		Run:   doJmxListCollected,
	}

	jmxListNotMatchingCmd = &cobra.Command{
		Use:   "not-matching",
		Short: "List attributes that don’t match any of your instances configuration.",
		Long:  ``,
		Run:   doJmxListNotCollected,
	}

	checks   = []string{}
	checkDir = "/etc/datadog-agent/conf.d"
)

func init() {
	// attach list command to jmx command
	jmxCmd.AddCommand(jmxListCmd)

	//attach list commands to list root
	jmxListCmd.AddCommand(jmxListEverythingCmd, jmxListMatchingCmd, jmxListLimitedCmd, jmxListCollectedCmd, jmxListNotMatchingCmd)

	jmxListCmd.PersistentFlags().StringSliceVar(&checks, "checks", []string{}, "JMX checks (required) (ex: jmx,tomcat)")
	jmxListCmd.MarkPersistentFlagRequired("checks")

	jmxListCmd.PersistentFlags().StringVarP(&checkDir, "confdir", "d", checkDir, "check configuration directory")

	// attach the command to the root
	AgentCmd.AddCommand(jmxCmd)
}

func doJmxListEverything(cmd *cobra.Command, args []string) {
	runJmxCommand("list_everything")
}

func doJmxListMatching(cmd *cobra.Command, args []string) {
	runJmxCommand("list_matching_attributes")
}

func doJmxListLimited(cmd *cobra.Command, args []string) {
	runJmxCommand("list_limited_attributes")
}

func doJmxListCollected(cmd *cobra.Command, args []string) {
	runJmxCommand("list_collected_attributes")
}

func doJmxListNotCollected(cmd *cobra.Command, args []string) {
	runJmxCommand("list_not_matching_attributes")
}

func runJmxCommand(command string) {
	runner := jmxfetch.New()

	runner.ReportOnConsole = true
	runner.Command = command
	runner.ConfDirectory = checkDir

	for _, c := range checks {
		checkFile := fmt.Sprintf("%v.d/conf.yaml", c)
		runner.Checks = append(runner.Checks, checkFile)
	}

	err := runner.Run()
	if err != nil {
		log.Fatalln(err)
	}

	err = runner.Wait()
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("JMXFetch exited sucessfully. If nothing was outputed please verify your \"checks\" and \"confdir\" flags or check JMXFetch log file.")
}
