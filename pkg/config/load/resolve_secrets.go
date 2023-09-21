package load

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/conf"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"gopkg.in/yaml.v2"
)

// ResolveSecrets merges all the secret values from origin into config. Secret values
// are identified by a value of the form "ENC[key]" where key is the secret key.
// See: https://github.com/DataDog/datadog-agent/blob/main/docs/agent/secrets.md
func ResolveSecrets(config conf.Config, origin string) error {
	// We have to init the secrets package before we can use it to decrypt
	// anything.
	secrets.Init(
		config.GetString("secret_backend_command"),
		config.GetStringSlice("secret_backend_arguments"),
		config.GetInt("secret_backend_timeout"),
		config.GetInt("secret_backend_output_max_size"),
		config.GetBool("secret_backend_command_allow_group_exec_perm"),
		config.GetBool("secret_backend_remove_trailing_line_break"),
	)

	if config.GetString("secret_backend_command") != "" {
		// Viper doesn't expose the final location of the file it
		// loads. Since we are searching for 'datadog.yaml' in multiple
		// locations we let viper determine the one to use before
		// updating it.
		yamlConf, err := yaml.Marshal(config.AllSettings())
		if err != nil {
			return fmt.Errorf("unable to marshal configuration to YAML to decrypt secrets: %v", err)
		}

		finalYamlConf, err := secrets.Decrypt(yamlConf, origin)
		if err != nil {
			return fmt.Errorf("unable to decrypt secret from datadog.yaml: %v", err)
		}
		r := bytes.NewReader(finalYamlConf)
		if err = config.MergeConfigOverride(r); err != nil {
			return fmt.Errorf("could not update main configuration after decrypting secrets: %v", err)
		}
	}
	return nil
}
