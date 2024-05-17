// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func exportRemoteConfig(fb flaretypes.FlareBuilder) error {
	// Dump the DB
	if err := getRemoteConfigDB(fb); err != nil {
		return err
	}

	// Dump the state
	token, err := security.FetchAuthToken(config.Datadog)
	if err != nil {
		return fmt.Errorf("couldn't get auth token: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}

	cli, err := agentgrpc.GetDDAgentSecureClient(ctx, ipcAddress, config.GetIPCPort())
	if err != nil {
		return err
	}
	in := new(emptypb.Empty)

	s, err := cli.GetConfigState(ctx, in)
	if err != nil {
		return fmt.Errorf("couldn't get the repositories state: %v", err)
	}

	var haState *pbgo.GetStateConfigResponse
	if config.Datadog.GetBool("multi_region_failover.enabled") {
		if haState, err = cli.GetConfigStateHA(ctx, in); err != nil {
			return fmt.Errorf("couldn't get the MRF repositories state: %v", err)
		}
	}

	err = fb.AddFileFromFunc("remote-config-state.log", func() ([]byte, error) {
		fct := func(w io.Writer) error {
			PrintRemoteConfigStates(w, s, haState)

			return nil
		}

		return functionOutputToBytes(fct), nil
	})
	if err != nil {
		return fmt.Errorf("couldn't add the remote-config-state.log file: %v", err)
	}

	return nil
}

func hashRCTargets(raw []byte) []byte {
	hash := sha256.Sum256(raw)
	// Convert it to readable hex
	s := hex.EncodeToString(hash[:])

	return []byte(s)
}

func getRemoteConfigDB(fb flaretypes.FlareBuilder) error {
	dstPath, _ := fb.PrepareFilePath("remote-config.db")
	tempPath, _ := fb.PrepareFilePath("remote-config.temp.db")
	srcPath := filepath.Join(config.Datadog.GetString("run_path"), "remote-config.db")

	// Copies the db so it avoids bbolt from being locked
	// Also avoid concurrent modifications
	err := util.CopyFileAll(srcPath, tempPath)
	// Delete the db at the end to avoid having target files content
	defer os.Remove(tempPath)
	if err != nil {
		// Prevent from having a clear db here
		return err
	}

	tempDB, err := bbolt.Open(tempPath, 0400, &bbolt.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tempDB.Close()
	dstDB, err := bbolt.Open(dstPath, 0600, &bbolt.Options{})
	if err != nil {
		return err
	}
	defer dstDB.Close()

	return tempDB.View(func(tempTx *bbolt.Tx) error {
		return tempTx.ForEach(func(bucketName []byte, tempBucket *bbolt.Bucket) error {
			return dstDB.Update(func(dstTx *bbolt.Tx) error {
				dstBucket, err := dstTx.CreateBucket(bucketName)
				if err != nil {
					return err
				}
				return tempBucket.ForEach(func(k, v []byte) error {
					if strings.HasSuffix(string(bucketName), "_targets") {
						return dstBucket.Put(k, hashRCTargets(v))
					}
					return dstBucket.Put(k, v)
				})
			})
		})
	})
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

// PrintRemoteConfigStates dump the whole remote-config state
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
}
