// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package rcservice

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/fatih/color"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// PrintRemoteConfigStates dumps the whole remote-config state to w.
func PrintRemoteConfigStates(w io.Writer, state *pbgo.GetStateConfigResponse, stateHA *pbgo.GetStateConfigResponse) {
	if state != nil {
		fmt.Fprintln(w, "\n=== Remote config DB state ===")
		printRemoteConfigStateContents(w, state)
	}

	if stateHA != nil {
		fmt.Fprintln(w, "\n=== Remote config HA DB state ===")
		printRemoteConfigStateContents(w, stateHA)
	}
}

func getStateString(state *pbgo.FileMetaState, padding int) string {
	if state == nil {
		return color.YellowString(fmt.Sprintf("%*s\n", padding, "- Not found"))
	}
	return fmt.Sprintf("%*s: %9d - Hash: %s\n", padding, "- Version", state.Version, state.Hash)
}

func printAndRemoveFile(w io.Writer, repo map[string]*pbgo.FileMetaState, name string, prefix string, padding int) {
	file, found := repo[name]
	fmt.Fprintf(w, "%s%s%s", prefix, name, getStateString(file, padding))
	if found {
		delete(repo, name)
	}
}

func printTUFRepo(w io.Writer, repo map[string]*pbgo.FileMetaState) {
	printAndRemoveFile(w, repo, "root.json", "", 20)
	printAndRemoveFile(w, repo, "timestamp.json", "|- ", 12)
	printAndRemoveFile(w, repo, "snapshot.json", "|- ", 13)
	printAndRemoveFile(w, repo, "targets.json", "|- ", 14)

	// Sort the keys to display the delegated targets in order
	keys := make([]string, 0, len(repo))
	for k := range repo {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		fmt.Fprintf(w, "    |- %s %s\n", name, getStateString(repo[name], 4))
	}
}

func printRemoteConfigStateContents(w io.Writer, state *pbgo.GetStateConfigResponse) {
	fmt.Fprintln(w, "\nConfiguration repository")
	fmt.Fprintln(w, strings.Repeat("-", 25))
	printTUFRepo(w, state.ConfigState)

	fmt.Fprintln(w, "\nDirector repository")
	fmt.Fprintln(w, strings.Repeat("-", 20))
	printTUFRepo(w, state.DirectorState)
	keys := make([]string, 0, len(state.TargetFilenames))
	for k := range state.TargetFilenames {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		fmt.Fprintf(w, "    |- %s - Hash: %s\n", name, state.TargetFilenames[name])
	}

	fmt.Fprintln(w, "\nRemote config active clients")
	fmt.Fprintln(w, strings.Repeat("-", 29))
	for _, client := range state.ActiveClients {
		fmt.Fprintf(w, "\n- Client %s\n%+v", client.Id, client)
		// Additional print of capabilities so it's more readable
		fmt.Fprintf(w, "\n    - Capabilities: ")
		for _, n := range client.Capabilities {
			fmt.Printf("% 08b", n)
		}
		fmt.Println("")
	}

	if len(state.ActiveClients) == 0 {
		fmt.Fprintln(w, "No active clients")
	}

	if len(state.ConfigSubscriptionStates) > 0 {
		fmt.Fprintln(w, "\nRemote config active subscriptions")
		fmt.Fprintln(w, strings.Repeat("-", 34))
		for _, subscription := range state.ConfigSubscriptionStates {
			fmt.Fprintf(w, "\n- Subscription %d\n", subscription.SubscriptionId)
			for _, trackedClient := range subscription.TrackedClients {
				fmt.Fprintf(w, "    - Tracked client %s - Seen any: %t - Products: %s\n",
					trackedClient.RuntimeId, trackedClient.SeenAny, trackedClient.Products.String(),
				)
			}
		}
	}
}
