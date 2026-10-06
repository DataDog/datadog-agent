// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ksmsharding implements 'agent ksm-sharding'.
package ksmsharding

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type predictionParams struct {
	count             int
	criteria          []string
	namespace         string
	resource          string
	colocatedResource string
	shardID           int
}

type prediction struct {
	Namespace    string           `json:"namespace"`
	GroupKind    string           `json:"group_kind"`
	HashResource string           `json:"hash_resource"`
	HashKey      sharding.HashKey `json:"hash_key"`
	OwnerShard   int              `json:"owner_shard"`
	OwnedByShard *bool            `json:"owned_by_shard,omitempty"`
}

type storesParams struct {
	output  io.Writer
	json    bool
	checkID string
}

// Commands returns the KSM sharding prediction and live-inspection commands.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	var jsonOutput bool
	root := &cobra.Command{
		Use:   "ksm-sharding",
		Short: "Predict KSM hash-shard ownership or inspect live stores",
	}
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Print JSON")

	params := predictionParams{shardID: -1}
	predict := &cobra.Command{
		Use:   "predict",
		Short: "Calculate ownership without contacting the running Agent or Kubernetes",
		Long: "Calculate ownership using the same hash-key and rendezvous algorithm as KSM. " +
			"Use the version-independent GroupKind (for example core/Pod or apps/Deployment), " +
			"and omit the namespace for cluster-scoped resources. This predicts assignment; it does not confirm collection.",
		Example: "  agent ksm-sharding predict --shard-count 4 --shard-criteria namespace,resource --namespace default --resource core/Pod",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := predictOwnership(params)
			if err != nil {
				return err
			}
			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), result)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAMESPACE\tRESOURCE\tHASH RESOURCE\tHASH KEY\tOWNER SHARD")
			fmt.Fprintf(w, "%s\t%s\t%s\t%q\t%d\n", namespaceName(result.Namespace), result.GroupKind, result.HashResource, result.HashKey, result.OwnerShard)
			if result.OwnedByShard != nil {
				fmt.Fprintf(w, "Owned by shard %d: %t\n", params.shardID, *result.OwnedByShard)
			}
			return w.Flush()
		},
	}
	predict.Flags().IntVar(&params.count, "shard-count", 0, "Number of shards")
	predict.Flags().StringSliceVar(&params.criteria, "shard-criteria", nil, "Hash dimensions: namespace, resource, or both")
	predict.Flags().StringVar(&params.namespace, "namespace", "", "Kubernetes namespace; empty for cluster-scoped resources")
	predict.Flags().StringVar(&params.resource, "resource", "", "Version-independent GroupKind, for example core/Pod")
	predict.Flags().StringVar(&params.colocatedResource, "colocated-resource", "", "First collector name in the resource's colocation group, for example deployments")
	predict.Flags().IntVar(&params.shardID, "shard-id", -1, "Also check ownership by this shard ID")

	storeParams := &storesParams{}
	stores := &cobra.Command{
		Use:   "stores",
		Short: "Inspect existing hash-sharded stores in the running Agent",
		Long: "List existing hash-sharded KSM stores and cached object counts in this Agent process. " +
			"Store existence and object counts do not confirm watch health. Eager collection is identified but its stores are not inventoried.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			storeParams.output = cmd.OutOrStdout()
			storeParams.json = jsonOutput
			return fxutil.OneShot(runStores,
				fx.Supply(storeParams),
				fx.Supply(command.GetDefaultCoreBundleParams(globalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	stores.Flags().StringVar(&storeParams.checkID, "check-id", "", "Inspect only this check instance ID")
	root.AddCommand(predict, stores)
	return []*cobra.Command{root}
}

func predictOwnership(params predictionParams) (prediction, error) {
	if params.count < 1 {
		return prediction{}, errors.New("shard-count must be positive")
	}
	if params.shardID < -1 || params.shardID >= params.count {
		return prediction{}, fmt.Errorf("shard-id must be between 0 and %d", params.count-1)
	}
	parts := strings.Split(params.resource, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return prediction{}, errors.New("resource must be a GroupKind such as core/Pod or apps/Deployment")
	}
	if len(params.criteria) == 0 {
		return prediction{}, errors.New("shard-criteria must include namespace, resource, or both")
	}
	seen := make(map[string]bool)
	for _, criterion := range params.criteria {
		if criterion != sharding.CriterionNamespace && criterion != sharding.CriterionResource {
			return prediction{}, fmt.Errorf("unknown shard criterion %q", criterion)
		}
		if seen[criterion] {
			return prediction{}, fmt.Errorf("duplicate shard criterion %q", criterion)
		}
		seen[criterion] = true
	}
	hashResource := params.resource
	if params.colocatedResource != "" {
		if !seen[sharding.CriterionResource] {
			return prediction{}, errors.New("colocated-resource requires resource sharding")
		}
		hashResource = params.colocatedResource
	}
	key := sharding.NewHashKey(params.criteria, params.namespace, hashResource)
	result := prediction{
		Namespace:    params.namespace,
		GroupKind:    params.resource,
		HashResource: hashResource,
		HashKey:      key,
		OwnerShard:   sharding.ShardResponsibleForKey(params.count, key),
	}
	if params.shardID >= 0 {
		owned := result.OwnerShard == params.shardID
		result.OwnedByShard = &owned
	}
	return result, nil
}

func runStores(_ log.Component, params *storesParams, client ipc.HTTPClient) error {
	endpoint, err := client.NewIPCEndpoint("/agent/ksm-sharding")
	if err != nil {
		return err
	}
	data, err := endpoint.DoGet()
	if err != nil {
		return fmt.Errorf("reading KSM stores from the running Agent: %w", err)
	}
	var snapshots []sharding.CheckSnapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return fmt.Errorf("decoding KSM store inventory: %w", err)
	}
	if params.checkID != "" {
		filtered := make([]sharding.CheckSnapshot, 0, 1)
		for _, snapshot := range snapshots {
			if snapshot.CheckID == params.checkID {
				filtered = append(filtered, snapshot)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("KSM check instance %q was not found", params.checkID)
		}
		snapshots = filtered
	}
	if params.json {
		return writeJSON(params.output, snapshots)
	}
	return printStores(params.output, snapshots)
}

func printStores(output io.Writer, snapshots []sharding.CheckSnapshot) error {
	if len(snapshots) == 0 {
		_, err := fmt.Fprintln(output, "No KSM checks are loaded in this Agent process.")
		return err
	}
	w := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	for _, snapshot := range snapshots {
		fmt.Fprintf(w, "%s: %s (shard %d of %d, criteria=%s)\n", snapshot.CheckID, snapshot.State, snapshot.ShardID, snapshot.ShardCount, strings.Join(snapshot.ShardCriteria, ","))
		if snapshot.State == "eager" {
			fmt.Fprintln(w, "  This check uses eager collection; hash-sharded store inspection does not apply.")
			continue
		}
		if len(snapshot.Stores) == 0 {
			fmt.Fprintln(w, "  No stores exist for this check at this snapshot.")
			continue
		}
		fmt.Fprintln(w, "NAMESPACE\tRESOURCE\tCOLLECTOR\tHASH KEY\tOWNER SHARD\tOBJECTS")
		for _, store := range snapshot.Stores {
			objects := "unknown"
			if store.Objects != nil {
				objects = strconv.Itoa(*store.Objects)
			}
			collector := store.Collector
			if collector == "" {
				collector = store.APIResource
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%q\t%d\t%s\n", namespaceName(store.Namespace), store.GroupKind, collector, store.HashKey, store.OwnerShard, objects)
		}
	}
	fmt.Fprintln(w, "Object counts describe cached data, not watch health.")
	return w.Flush()
}

func namespaceName(namespace string) string {
	if namespace == "" {
		return "<cluster>"
	}
	return namespace
}

func writeJSON(output io.Writer, value interface{}) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
