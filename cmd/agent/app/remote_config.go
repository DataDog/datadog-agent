// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	remoteConfigCommand = &cobra.Command{
		Use:    "remote-config",
		Short:  "Remote configuration management command line",
		Long:   ``,
		Hidden: true,
	}

	remoteConfigGetCommand = &cobra.Command{
		Use:   "get",
		Short: "retrieve configurations of a product",
		Long:  ``,
		RunE:  remoteConfigGetConfigurations,
	}

	remoteConfigDumpCommand = &cobra.Command{
		Use:   "dump-store",
		Short: "dump store",
		Long:  ``,
		RunE:  remoteConfigDumpStore,
	}

	remoteConfigGetArgs = struct {
		product    string
		org        int
		datacenter string
	}{}
)

func replaceRaw(obj interface{}) interface{} {
	switch obj := obj.(type) {
	case map[string]interface{}:
		for k, v := range obj {
			if bytes, ok := v.(string); ok && k == "raw" {
				decoded, _ := base64.RawStdEncoding.DecodeString(bytes)
				obj[k] = string(decoded)
			} else {
				replaceRaw(v)
			}
		}
		return obj
	case []interface{}:
		for _, v := range obj {
			replaceRaw(v)
		}
		return obj
	default:
		return obj
	}

}

func remoteConfigDumpStore(_ *cobra.Command, _ []string) error {
	config.DetectFeatures()

	configService, err := service.NewService(service.Opts{
		ReadOnly:               true,
		RemoteConfigurationKey: fmt.Sprintf("%s/%d/123", remoteConfigGetArgs.datacenter, remoteConfigGetArgs.org),
	})
	if err != nil {
		return err
	}

	store := configService.GetStore()

	var errs *multierror.Error
	productConfigs := make(map[string][]interface{})
	for _, product := range pbgo.Product_name {
		configs, err := store.GetConfigs(product)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get config for product '%s': %s", product, err)
			continue
		}

		if err == nil {
			productConfigs[product] = nil
			for _, config := range configs {
				content, err := json.Marshal(config)
				if err != nil {
					errs = multierror.Append(errs, err)
				}

				m := make(map[string]interface{})
				if err := json.Unmarshal(content, &m); err != nil {
					errs = multierror.Append(errs, err)
				}

				productConfigs[product] = append(productConfigs[product], replaceRaw(m))
			}
		}
	}

	data, err := json.MarshalIndent(productConfigs, "", "  ")
	if err != nil {
		errs = multierror.Append(errs, err)
		return errs
	}

	fmt.Println(string(data))

	return errs.ErrorOrNil()
}

func remoteConfigGetConfigurations(_ *cobra.Command, _ []string) error {
	// Prevent autoconfig to run when running status as it logs before logger is setup
	// Cannot rely on config.Override as env detection is run before overrides are set
	os.Setenv("DD_AUTOCONFIG_FROM_ENVIRONMENT", "false")
	err := common.SetupConfigWithoutSecrets(confFilePath, "")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return err
	}

	var errs *multierror.Error
	var productConfigs []interface{}
	configs, err := getConfigs(remoteConfigGetArgs.product)
	if err != nil {
		return err
	}
	for _, config := range configs {
		content, err := json.Marshal(config)
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}

		m := make(map[string]interface{})
		if err := json.Unmarshal(content, &m); err != nil {
			errs = multierror.Append(errs, err)
			continue
		}

		productConfigs = append(productConfigs, replaceRaw(m))
	}

	data, err := json.MarshalIndent(productConfigs, "", "  ")
	if err != nil {
		errs = multierror.Append(errs, err)
		return errs
	}

	fmt.Println(string(data))

	return errs.ErrorOrNil()
}

// getConfigs returns all the configurations for a product
func getConfigs(product string) ([]*pbgo.ConfigResponse, error) {
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	conn, err := grpc.DialContext(
		context.Background(),
		fmt.Sprintf("%s:%v", ipcAddress, config.Datadog.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, err
	}

	agentClient := pbgo.NewAgentSecureClient(conn)

	token, err := security.FetchAuthToken()
	if err != nil {
		err = fmt.Errorf("unable to fetch authentication token: %w", err)
		log.Infof("unable to establish stream, will possibly retry: %s", err)
		return nil, err
	}

	ctx, cancel := context.WithCancel(
		metadata.NewOutgoingContext(context.Background(), metadata.MD{
			"authorization": []string{fmt.Sprintf("Bearer %s", token)},
		}),
	)
	defer cancel()

	request := pbgo.GetConfigsRequest{
		Product: pbgo.Product(pbgo.Product_value[product]),
	}
	response, err := agentClient.GetConfigs(ctx, &request)
	if err != nil {
		return nil, err
	}

	return response.ConfigResponses, nil
}

func init() {
	AgentCmd.AddCommand(remoteConfigCommand)
	remoteConfigCommand.AddCommand(remoteConfigGetCommand)
	remoteConfigCommand.AddCommand(remoteConfigDumpCommand)

	remoteConfigGetCommand.Flags().StringVar(&remoteConfigGetArgs.product, "product", "", "product name")
	remoteConfigDumpCommand.Flags().IntVar(&remoteConfigGetArgs.org, "org", 0, "organization id")
	remoteConfigDumpCommand.Flags().StringVar(&remoteConfigGetArgs.datacenter, "datacenter", "datadoghq.com", "site")
	remoteConfigGetCommand.MarkFlagRequired("product") //nolint:errcheck
}
