// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/fx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

const fetchRemoteCommandsTimeout = 500 * time.Millisecond

// reservedShorthands contains the single-letter shorthands used by the root
// command's persistent flags. Remote command flags must not reuse these.
var reservedShorthands = map[string]bool{}

// initReservedShorthands records the persistent flag shorthands from the root
// command so that registerFlags can avoid conflicts.
func initReservedShorthands(root *cobra.Command) {
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Shorthand != "" {
			reservedShorthands[f.Shorthand] = true
		}
	})
}

// fetchRemoteCommands attempts a fast connection to the running Core Agent
// to retrieve remote command definitions. Returns nil if the agent isn't
// running or the fetch times out.
func fetchRemoteCommands() []*pb.RemoteAgentCommandGroup {
	// Set up config search paths so ReadInConfig can find the YAML file.
	// The Fx config setup normally does this, but we run before Fx here.
	cfgBuilder := pkgconfigsetup.GlobalConfigBuilder()
	cfgBuilder.SetConfigName(ConfigName)
	// Honor -c/--cfgpath first if provided, so it takes priority over
	// the default path. This is important for dev builds where the config
	// is in a non-default location (e.g., bin/agent/dist).
	if confPath := extractConfPath(os.Args[1:]); confPath != "" {
		cfgBuilder.AddConfigPath(confPath)
	}
	cfgBuilder.AddConfigPath(defaultpaths.GetDefaultConfPath())

	cfg := pkgconfigsetup.Datadog()
	// Attempt to load the YAML config so ConfigFileUsed() resolves,
	// which is needed to locate the auth token and IPC cert files.
	// Errors are expected (no config file, agent not installed, etc.)
	// and silently ignored.
	if err := cfg.ReadInConfig(); err != nil {
		return nil
	}

	token, err := security.FetchAuthToken(cfg)
	if err != nil {
		return nil
	}

	tlsClientConfig, _, _, err := cert.FetchIPCCert(cfg)
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), fetchRemoteCommandsTimeout)
	defer cancel()

	ipcAddr, err := pkgconfigsetup.GetIPCAddress(cfg)
	if err != nil {
		return nil
	}
	ipcPort := pkgconfigsetup.GetIPCPort()

	client, err := agentgrpc.GetDDAgentSecureClient(ctx, ipcAddr, ipcPort, tlsClientConfig)
	if err != nil {
		return nil
	}

	md := metadata.MD{"authorization": []string{"Bearer " + token}}
	ctx = metadata.NewOutgoingContext(ctx, md)

	resp, err := client.ListRemoteCommands(ctx, &emptypb.Empty{})
	if err != nil {
		return nil
	}
	return resp.AgentCommands
}

// buildCobraCommand creates a cobra.Command from a remote command proto definition.
// The command's RunE bootstraps Fx to get IPC credentials, then calls ExecuteRemoteCommand.
func buildCobraCommand(cmd *pb.Command, globalParams *GlobalParams) *cobra.Command {
	// Use the alias as the cobra command name when present, so that
	// `agent <alias>` is recognized by cobra. The real command name
	// is still sent to the Core Agent in ExecuteRemoteCommand.
	useName := cmd.Name
	if cmd.Alias != "" {
		useName = cmd.Alias
	}

	cobraCmd := &cobra.Command{
		Use:   useName,
		Short: cmd.Helper,
		Long:  cmd.LongDescription,
	}

	// When the command has an alias, also register the real name as a
	// cobra alias so both `agent <alias>` and `agent <name>` work.
	if cmd.Alias != "" && cmd.Name != cmd.Alias {
		cobraCmd.Aliases = []string{cmd.Name}
	}

	// Register flags from parameter definitions and collect storage pointers
	// so RunE can read cobra-parsed values.
	flagStore := registerFlags(cobraCmd, cmd.Parameters)

	// Only set RunE if this command is meant to be executed (has parameters or no children).
	// Parent-only grouping commands just hold children.
	cobraCmd.RunE = func(_ *cobra.Command, args []string) error {
		return executeViaAgent(globalParams, cmd.Name, flagStore, cmd.Parameters, args)
	}

	// Recursively build children
	for _, child := range cmd.Children {
		cobraCmd.AddCommand(buildCobraCommand(child, globalParams))
	}

	return cobraCmd
}

// flagValues stores pointers to flag values registered on a cobra command,
// keyed by parameter name.
type flagValues struct {
	strings       map[string]*string
	int64s        map[string]*int64
	uint64s       map[string]*uint64
	float64s      map[string]*float64
	bools         map[string]*bool
	stringSlices  map[string]*[]string
	int64Slices   map[string]*[]int64
	uintSlices    map[string]*[]uint
	float64Slices map[string]*[]float64
}

func newFlagValues() *flagValues {
	return &flagValues{
		strings:       make(map[string]*string),
		int64s:        make(map[string]*int64),
		uint64s:       make(map[string]*uint64),
		float64s:      make(map[string]*float64),
		bools:         make(map[string]*bool),
		stringSlices:  make(map[string]*[]string),
		int64Slices:   make(map[string]*[]int64),
		uintSlices:    make(map[string]*[]uint),
		float64Slices: make(map[string]*[]float64),
	}
}

// toStruct converts the parsed flag values into a structpb.Struct,
// including only flags that were explicitly set and positional args.
func (fv *flagValues) toStruct(params []*pb.CommandParameter, positionalArgs []string) (*structpb.Struct, error) {
	fields := make(map[string]interface{})

	for name, v := range fv.strings {
		if v != nil && *v != "" {
			fields[name] = *v
		}
	}
	for name, v := range fv.int64s {
		if v != nil {
			fields[name] = float64(*v)
		}
	}
	for name, v := range fv.uint64s {
		if v != nil {
			fields[name] = float64(*v)
		}
	}
	for name, v := range fv.float64s {
		if v != nil {
			fields[name] = *v
		}
	}
	for name, v := range fv.bools {
		if v != nil && *v {
			fields[name] = *v
		}
	}
	for name, v := range fv.stringSlices {
		if v != nil && len(*v) > 0 {
			items := make([]interface{}, len(*v))
			for i, s := range *v {
				items[i] = s
			}
			fields[name] = items
		}
	}
	for name, v := range fv.int64Slices {
		if v != nil && len(*v) > 0 {
			items := make([]interface{}, len(*v))
			for i, n := range *v {
				items[i] = float64(n)
			}
			fields[name] = items
		}
	}
	for name, v := range fv.uintSlices {
		if v != nil && len(*v) > 0 {
			items := make([]interface{}, len(*v))
			for i, n := range *v {
				items[i] = float64(n)
			}
			fields[name] = items
		}
	}
	for name, v := range fv.float64Slices {
		if v != nil && len(*v) > 0 {
			items := make([]interface{}, len(*v))
			for i, n := range *v {
				items[i] = n
			}
			fields[name] = items
		}
	}

	// Add positional args matched to non-flag parameters
	posIdx := 0
	for _, p := range params {
		if p.IsFlag {
			continue
		}
		if posIdx < len(positionalArgs) {
			fields[p.Name] = positionalArgs[posIdx]
			posIdx++
		}
	}

	if len(fields) == 0 {
		return nil, nil
	}
	return structpb.NewStruct(fields)
}

// registerFlags registers cobra flags for each parameter definition and returns
// a flagValues struct holding pointers to the parsed values.
func registerFlags(cmd *cobra.Command, params []*pb.CommandParameter) *flagValues {
	fv := newFlagValues()

	for _, p := range params {
		if !p.IsFlag {
			continue
		}

		name := p.Name
		short := p.ShortName
		usage := p.Helper
		flags := cmd.Flags()
		if p.IsPersistent {
			flags = cmd.PersistentFlags()
		}

		// Drop the shorthand if it conflicts with a persistent flag from
		// the root command (e.g., -c for --cfgpath, -n for --no-color).
		if short != "" && reservedShorthands[short] {
			short = ""
		}

		d := p.DefaultValue // nil when no default is set

		switch p.Type {
		case pb.ParameterType_PARAMETER_TYPE_STRING, pb.ParameterType_PARAMETER_TYPE_UNSPECIFIED:
			v := new(string)
			def := ""
			if d != nil {
				def = d.StringDefault
			}
			flags.StringVarP(v, name, short, def, usage)
			fv.strings[name] = v
		case pb.ParameterType_PARAMETER_TYPE_INT:
			v := new(int64)
			var def int64
			if d != nil {
				def = d.IntDefault
			}
			flags.Int64VarP(v, name, short, def, usage)
			fv.int64s[name] = v
		case pb.ParameterType_PARAMETER_TYPE_UINT:
			v := new(uint64)
			var def uint64
			if d != nil {
				def = d.UintDefault
			}
			flags.Uint64VarP(v, name, short, def, usage)
			fv.uint64s[name] = v
		case pb.ParameterType_PARAMETER_TYPE_FLOAT:
			v := new(float64)
			var def float64
			if d != nil {
				def = d.FloatDefault
			}
			flags.Float64VarP(v, name, short, def, usage)
			fv.float64s[name] = v
		case pb.ParameterType_PARAMETER_TYPE_BOOL:
			v := new(bool)
			def := false
			if d != nil {
				def = d.BoolDefault
			}
			flags.BoolVarP(v, name, short, def, usage)
			fv.bools[name] = v
		case pb.ParameterType_PARAMETER_TYPE_STRING_SLICE:
			v := new([]string)
			var def []string
			if d != nil && len(d.StringSliceDefault) > 0 {
				def = d.StringSliceDefault
			}
			flags.StringSliceVarP(v, name, short, def, usage)
			fv.stringSlices[name] = v
		case pb.ParameterType_PARAMETER_TYPE_INT_SLICE:
			v := new([]int64)
			var def []int64
			if d != nil && len(d.IntSliceDefault) > 0 {
				def = d.IntSliceDefault
			}
			flags.Int64SliceVarP(v, name, short, def, usage)
			fv.int64Slices[name] = v
		case pb.ParameterType_PARAMETER_TYPE_UINT_SLICE:
			v := new([]uint)
			var def []uint
			if d != nil && len(d.UintSliceDefault) > 0 {
				for _, val := range d.UintSliceDefault {
					def = append(def, uint(val))
				}
			}
			flags.UintSliceVarP(v, name, short, def, usage)
			fv.uintSlices[name] = v
		case pb.ParameterType_PARAMETER_TYPE_FLOAT_SLICE:
			v := new([]float64)
			var def []float64
			if d != nil && len(d.FloatSliceDefault) > 0 {
				def = d.FloatSliceDefault
			}
			flags.Float64SliceVarP(v, name, short, def, usage)
			fv.float64Slices[name] = v
		}

		if p.Required {
			_ = cobra.MarkFlagRequired(flags, name)
		}
	}

	return fv
}

// executeViaAgent bootstraps Fx to get IPC credentials, then calls ExecuteRemoteCommand.
func executeViaAgent(globalParams *GlobalParams, commandPath string, fv *flagValues, params []*pb.CommandParameter, args []string) error {
	return fxutil.OneShot(func(_ log.Component, _ config.Component, ipcComp ipc.Component) error {
		arguments, err := fv.toStruct(params, args)
		if err != nil {
			return fmt.Errorf("failed to build arguments: %w", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		md := metadata.MD{"authorization": []string{"Bearer " + ipcComp.GetAuthToken()}}
		ctx = metadata.NewOutgoingContext(ctx, md)

		ipcAddr, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
		if err != nil {
			return err
		}

		client, err := agentgrpc.GetDDAgentSecureClient(ctx, ipcAddr, pkgconfigsetup.GetIPCPort(), ipcComp.GetTLSClientConfig())
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not connect to the Datadog Agent. Is the agent running?")
			return err
		}

		req := &pb.ExecuteCommandRequest{
			CommandPath: commandPath,
			Arguments:   arguments,
		}

		resp, err := client.ExecuteRemoteCommand(ctx, req)
		if err != nil {
			return formatRemoteCommandError(commandPath, err)
		}

		if resp.Stdout != "" {
			fmt.Print(resp.Stdout)
		}
		if resp.Stderr != "" {
			fmt.Fprint(os.Stderr, resp.Stderr)
		}
		if resp.ExitCode != 0 {
			os.Exit(int(resp.ExitCode))
		}

		return nil
	},
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
			LogParams:    log.ForOneShot(LoggerName, "off", true),
		}),
		core.Bundle(),
		ipcfx.ModuleReadOnly(),
	)
}

// extractConfPath scans raw args for -c/--cfgpath and returns its value.
func extractConfPath(args []string) string {
	for i, arg := range args {
		if (arg == "-c" || arg == "--cfgpath") && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "-c=") {
			return strings.TrimPrefix(arg, "-c=")
		}
		if strings.HasPrefix(arg, "--cfgpath=") {
			return strings.TrimPrefix(arg, "--cfgpath=")
		}
	}
	return ""
}

// formatRemoteCommandError produces a human-friendly error message from a
// remote command execution failure.
func formatRemoteCommandError(commandPath string, err error) error {
	errorStr := err.Error()
	if s, ok := grpcstatus.FromError(err); ok {
		switch s.Code() {
		case codes.Unimplemented:
			errorStr = "command not implemented by remote agent"
		case codes.DeadlineExceeded:
			errorStr = "timed out waiting for command"
		default:
			errorStr = s.Message()
		}
	}
	return fmt.Errorf("failed to execute remote command %q due to an internal error.\n\nDetails: %s", commandPath, errorStr)
}

// removeSubcommand removes a subcommand by name from a parent command.
func removeSubcommand(parent *cobra.Command, name string) {
	var remaining []*cobra.Command
	for _, child := range parent.Commands() {
		if child.Name() != name {
			remaining = append(remaining, child)
		}
	}
	parent.ResetCommands()
	for _, child := range remaining {
		parent.AddCommand(child)
	}
}
