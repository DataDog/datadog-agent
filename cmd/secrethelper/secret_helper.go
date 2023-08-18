// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets

// Package secrethelper implements the secrethelper subcommand.
//
// This subcommand is shared between multiple agent binaries.
//
// It provides a "read" command to fetch secrets. It can be used in 2 different ways:
//
// 1) With the "--with-provider-prefixes" option enabled. Each input secret
// should follow this format: "providerPrefix/some/path". The provider prefix
// indicates where to fetch the secrets from. At the moment, we support "file"
// and "k8s_secret". The path can mean different things depending on the
// provider. In "file" it's a file system path. In "k8s_secret", it follows this
// format: "namespace/name/key".
//
// 2) Without the "--with-provider-prefixes" option. The program expects a root
// path in the arguments and input secrets are just paths relative to the root
// one. So for example, if the secret is "my_secret" and the root path is
// "/some/path", the fetched value of the secret will be the contents of
// "/some/path/my_secret". This option was offered before introducing
// "--with-provider-prefixes" and is kept to avoid breaking compatibility.
package secrethelper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/cmd/secrethelper/providers"
	s "github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

const (
	providerPrefixesFlag    = "with-provider-prefixes"
	providerPrefixSeparator = "@"
	filePrefix              = "file"
	k8sSecretPrefix         = "k8s_secret"
)

// NewKubeClient TODO <agent-core>
type NewKubeClient func(timeout time.Duration) (kubernetes.Interface, error)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	usePrefixes bool

	// args are the positional command-line arguments
	args []string
}

// Commands returns a slice of subcommands of the parent command.
func Commands() []*cobra.Command {
	cliParams := &cliParams{}
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read secrets",
		Long:  ``,
		Args:  cobra.MaximumNArgs(1), // 0 when using the provider prefixes option, 1 when reading a file
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			return fxutil.OneShot(readCmd,
				fx.Supply(cliParams),
			)
		},
	}
	cmd.PersistentFlags().BoolVarP(&cliParams.usePrefixes, providerPrefixesFlag, "", false, "Use prefixes to select the secrets provider (file, k8s_secret)")

	secretHelperCmd := &cobra.Command{
		Use:   "secret-helper",
		Short: "Secret management provider helper",
		Long:  ``,
	}
	secretHelperCmd.AddCommand(cmd)

	return []*cobra.Command{secretHelperCmd}
}

type secretsRequest struct {
	Version string   `json:"version"`
	Secrets []string `json:"secrets"`
}

func readCmd(cliParams *cliParams) error {
	dir := ""
	if len(cliParams.args) == 1 {
		dir = cliParams.args[0]
	}

	return readSecrets(os.Stdin, os.Stdout, dir, cliParams.usePrefixes, apiserver.GetKubeClient)
}

func readSecrets(r io.Reader, w io.Writer, dir string, usePrefixes bool, newKubeClientFunc NewKubeClient) error {
	inputSecrets, err := parseInputSecrets(r)
	if err != nil {
		return err
	}

	if usePrefixes {
		return writeFetchedSecrets(w, readSecretsUsingPrefixes(inputSecrets, dir, newKubeClientFunc))
	}

	return writeFetchedSecrets(w, readSecretsFromFile(inputSecrets, dir))
}

func parseInputSecrets(r io.Reader) ([]string, error) {
	in, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var request secretsRequest
	err = json.Unmarshal(in, &request)
	if err != nil {
		return nil, errors.New("failed to unmarshal json input")
	}

	version := splitVersion(request.Version)
	compatVersion := splitVersion(s.PayloadVersion)
	if version[0] != compatVersion[0] {
		return nil, fmt.Errorf("incompatible protocol version %q", request.Version)
	}

	if len(request.Secrets) == 0 {
		return nil, errors.New("no secrets listed in input")
	}

	return request.Secrets, nil
}

func writeFetchedSecrets(w io.Writer, fetchedSecrets map[string]s.Secret) error {
	out, err := json.Marshal(fetchedSecrets)
	if err != nil {
		return err
	}

	_, err = w.Write(out)
	return err
}

func readSecretsFromFile(secrets []string, dir string) map[string]s.Secret {
	res := make(map[string]s.Secret)

	for _, secretID := range secrets {
		res[secretID] = providers.ReadSecretFile(filepath.Join(dir, secretID))
	}

	return res
}

func readSecretsUsingPrefixes(secrets []string, rootPath string, newKubeClientFunc NewKubeClient) map[string]s.Secret {
	res := make(map[string]s.Secret)

	for _, secretID := range secrets {
		prefix, id, err := parseSecretWithPrefix(secretID, rootPath)
		if err != nil {
			res[secretID] = s.Secret{Value: "", ErrorMsg: err.Error()}
			continue
		}

		switch prefix {
		case filePrefix:
			res[secretID] = providers.ReadSecretFile(id)
		case k8sSecretPrefix:
			kubeClient, err := newKubeClientFunc(10 * time.Second)
			if err != nil {
				res[secretID] = s.Secret{Value: "", ErrorMsg: err.Error()}
			} else {
				res[secretID] = providers.ReadKubernetesSecret(kubeClient, id)
			}
		default:
			res[secretID] = s.Secret{Value: "", ErrorMsg: fmt.Sprintf("provider not supported: %s", prefix)}
		}
	}

	return res
}

func parseSecretWithPrefix(secretID string, rootPath string) (prefix string, id string, err error) {
	split := strings.SplitN(secretID, providerPrefixSeparator, 2)

	// This is to make the migration from "readsecret.sh" (without
	// "--with-provider-prefixes") to "readsecret_multiple_providers.sh" (uses
	// "--with-provider-prefixes") easier.
	// To avoid forcing users to change all their secrets at once, we have
	// decided that we'll handle secrets without the prefix separator as we
	// handle them when the "--with-provider-prefixes" is disabled. That is,
	// they should be fetched from the file system, and they specify a path
	// that's relative to the one specified in the first arg when calling the
	// program (rootPath arg of this func).
	if len(split) < 2 {
		prefix = filePrefix
		id, err = filepath.Abs(filepath.Join(rootPath, secretID))
		return prefix, id, err
	}

	prefix, id = split[0], split[1]
	return prefix, id, nil
}

func splitVersion(ver string) []string {
	return strings.SplitN(ver, ".", 2)
}
