// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package capture

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MakeCommand returns the capture subcommand for the 'trace-agent' command.
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	return &cobra.Command{
		Use:   "capture",
		Short: "Capture live tracer payloads.",
		Long:  `Use this to capture live tracer payloads received by the trace-agent.`,
		RunE: func(*cobra.Command, []string) error {
			return fxutil.OneShot(capture,
				config.Module,
				fx.Supply(coreconfig.NewAgentParams(globalParamsGetter().ConfPath)),
				fx.Supply(secrets.NewEnabledParams()),
				coreconfig.Module,
				secretsimpl.Module,
			)
		},
	}
}

func capture(config config.Component) error {
	tracecfg := config.Object()
	if tracecfg == nil {
		return errors.New("cannot parse config")
	}
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/capture", tracecfg.DebugServerPort))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
