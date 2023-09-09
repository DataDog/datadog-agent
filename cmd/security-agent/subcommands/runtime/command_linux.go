// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/reporter"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	runtimeCmd := &cobra.Command{
		Use:   "runtime",
		Short: "runtime Agent utility commands",
	}

	runtimeCmd.AddCommand(commonPolicyCommands(globalParams)...)
	runtimeCmd.AddCommand(selfTestCommands(globalParams)...)
	runtimeCmd.AddCommand(activityDumpCommands(globalParams)...)
	runtimeCmd.AddCommand(securityProfileCommands(globalParams)...)
	runtimeCmd.AddCommand(processCacheCommands(globalParams)...)
	runtimeCmd.AddCommand(networkNamespaceCommands(globalParams)...)
	runtimeCmd.AddCommand(discardersCommands(globalParams)...)

	// Deprecated
	runtimeCmd.AddCommand(checkPoliciesCommands(globalParams)...)
	runtimeCmd.AddCommand(reloadPoliciesCommands(globalParams)...)

	return []*cobra.Command{runtimeCmd}
}


func printSecurityActivityDumpMessage(prefix string, msg *api.ActivityDumpMessage) {
	fmt.Printf("%s- name: %s\n", prefix, msg.GetMetadata().GetName())
	fmt.Printf("%s  start: %s\n", prefix, msg.GetMetadata().GetStart())
	fmt.Printf("%s  timeout: %s\n", prefix, msg.GetMetadata().GetTimeout())
	if len(msg.GetMetadata().GetComm()) > 0 {
		fmt.Printf("%s  comm: %s\n", prefix, msg.GetMetadata().GetComm())
	}
	if len(msg.GetMetadata().GetContainerID()) > 0 {
		fmt.Printf("%s  container ID: %s\n", prefix, msg.GetMetadata().GetContainerID())
	}
	if len(msg.GetTags()) > 0 {
		fmt.Printf("%s  tags: %s\n", prefix, strings.Join(msg.GetTags(), ", "))
	}
	fmt.Printf("%s  differentiate args: %v\n", prefix, msg.GetMetadata().GetDifferentiateArgs())
	printActivityTreeStats(prefix, msg.GetStats())
	if len(msg.GetStorage()) > 0 {
		fmt.Printf("%s  storage:\n", prefix)
		for _, storage := range msg.GetStorage() {
			printStorageRequestMessage(prefix+"\t", storage)
		}
	}
}