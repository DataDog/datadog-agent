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

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/fatih/color"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func exportRemoteConfig(fb flaretypes.FlareBuilder) error {
	// Dump the DB
	if err := getRemoteConfigDB(fb); err != nil {
		return err
	}

	// Dump the state
	token, err := security.FetchAuthToken()
	if err != nil {
		return fmt.Errorf("Couldn't get auth token: %v", err)
	}
	ctx, close := context.WithCancel(context.Background())
	defer close()
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	cli, err := agentgrpc.GetDDAgentSecureClient(ctx)
	if err != nil {
		return err
	}
	in := new(emptypb.Empty)

	s, err := cli.GetConfigState(ctx, in)
	if err != nil {
		return fmt.Errorf("Couldn't get the repositories state: %v", err)
	}

	fb.AddFileFromFunc("remote-config-state.log", func() ([]byte, error) {
		fct := func(w io.Writer) error {
			PrintRemoteConfigState(w, s)

			return nil
		}

		return functionOutputToBytes(fct), nil
	})

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

// PrintRemoteConfigState dump the whole remote-config state
func PrintRemoteConfigState(w io.Writer, s *pbgo.GetStateConfigResponse) {
	fmt.Fprintln(w, "\n=== Remote config DB state ===")

	fmt.Fprintln(w, "\nConfiguration repository")
	fmt.Fprintln(w, strings.Repeat("-", 25))
	printTUFRepo(w, s.ConfigState)

	fmt.Fprintln(w, "\nDirector repository")
	fmt.Fprintln(w, strings.Repeat("-", 20))
	printTUFRepo(w, s.DirectorState)
	keys := make([]string, 0, len(s.TargetFilenames))
	for k := range s.TargetFilenames {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		fmt.Fprintf(w, "    |- %s - Hash: %s\n", name, s.TargetFilenames[name])
	}

	fmt.Fprintln(w, "\n=== Remote config active clients ===")

	for _, client := range s.ActiveClients {
		fmt.Fprintf(w, "\n== Client %s ==\n%+v\n\tCapabilities: ", client.Id, client)
		// Additional print of capabilities so it's more readable
		for _, n := range client.Capabilities {
			fmt.Printf("% 08b", n)
		}
		fmt.Println("")
	}

	if len(s.ActiveClients) == 0 {
		fmt.Fprintln(w, "No active clients")
	}
}
