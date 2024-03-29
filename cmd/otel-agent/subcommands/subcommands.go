// Package subcommands contains the subcommands of the otel-agent.
package subcommands

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfPath   string
	ConfigName string
	LoggerName string
}
