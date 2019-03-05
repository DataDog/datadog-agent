# [BETA] Secrets Management

**This feature is in beta and its options or behavior might break between
minor or bugfix releases of the Agent.**

This feature is only available on Linux at the moment.

Starting with version `6.3.0`, the agent is able to leverage the `secrets`
package in order to call a user-provided executable to handle retrieval or
decryption of secrets, which are then loaded in memory by the agent. This
feature allows users to no longer store passwords and other secrets in plain
text in configuration files. Users have the flexibility to design their
executable according to their preferred key management service, authentication
method, and continuous integration workflow.

For now, secrets are not supported in APM or Live Process configurations.

This section covers how to set up this feature.

* [Defining secrets in configurations](#defining-secrets-in-configurations)
* [Retrieving secrets from the secret backend](#retrieving-secrets-from-the-secret-backend)
  * [Configuration](#configuration)
  * [Agent security requirements](#agent-security-requirements)
    * [Linux](#linux)
  * [The executable API](#the-executable-api)
* [Troubleshooting](#troubleshooting)
  * [Seeing configurations after secrets were injected](#seeing-configurations-after-secrets-were-injected)
  * [Debugging your secret_backend_command](#debugging-your-secret_backend_command)
    * [Linux](#linux-1)
  * [Agent refusing to start](#agent-refusing-to-start)

## Defining secrets in configurations

To declare a secret in a check configuration simply use the `ENC[]` notation.
This notation can be used to denotate as a secret the *value* of any YAML field
in your configuration (not the key), in any section (`init_config`, `instances`,
`logs`, ...).

Secrets are supported in every configuration backend: file, etcd, consul ...

Starting version `6.10.0`, secrets are supported in environment variables.

Secrets are also supported in `datadog.yaml`. The agent will first load the
main configuration and reload it after decrypting the secrets. This means the
only place where secrets can't be used is the `secret_*` settings (see
Configuration section).

Secrets are always strings, this means you can't use this feature to set the
value of a setting of type integer or boolean (such as `GUI_port` for example).

Example:

```yaml
instances:
  - server: db_prod
    # two valid secret handles
    user: "ENC[db_prod_user]"
    password: "ENC[db_prod_password]"

    # The `ENC[]` handle must contain the entire YAML value, which means that
    # the following will NOT be detected as a secret handle:
    password2: "db-ENC[prod_password]"
```

In the above example we have two secrets : `db_prod_user` and
`db_prod_password`. Those are the secrets **handles** and each must uniquely
identify a secret within your secrets management backend.

Between the brackets any character is allowed as long as the YAML configuration
is valid. This means you could use any format you want.

Example 1 (be careful to escape quotes so your YAML file is valid):

```
"ENC[{\"env\": \"prod\", \"check\": \"postgres\", \"id\": \"user_password\", \"az\": \"us-east-1a\"}]"
```

In this example the secret's handle is the string `{"env": "prod", "check":
"postgres", "id": "user_password", "az": "us-east-1a"}`.

Example 2:

```
"ENC[AES256_GCM,data:v8jQ=,iv:HBE=,aad:21c=,tag:gA==]"
```

In this example the secret handle is the string `AES256_GCM,data:v8jQ=,iv:HBE=,aad:21c=,tag:gA==`.

Example 3:

There is no need to escape inner `[` and `]`. The agent will select everything between the first `ENC[` and the last `]`.

```yaml
instances:
  - server: db_prod
    user: "ENC[user_array[1337]]"
```

In this example the secret handle is the string `user_array[1337]`.

**Autodiscovery**:

Secrets are resolved **after**
[Autodiscovery](https://docs.datadoghq.com/agent/autodiscovery/?tab=docker)
template variables. This means you can use them in a secret handle to fetch
secrets specific to a container.

Example:

```yaml
instances:
  - server: %%host%%
    user: ENC[db_prod_user_%%host%%]
    password: ENC[db_prod_password_%%host%%]
```

## Retrieving secrets from the secret backend

To retrieve secrets, you have to provide an executable that is able to
authenticate to and fetch secrets from your secrets management backend.

The agent will cache secrets internally in memory to reduce the number of calls
(useful in a containerized environment for example). The agent calls the
executable every time it accesses a check configuration file that contains at
least one secret handle for which the secret is not already loaded in memory. In
particular, secrets that have already been loaded in memory do not trigger
additional calls to the executable. In practice, this means that the agent calls
the user-provided executable once per file that contains a secret handle at
startup, and might make additional calls to the executable later if the agent or
instance is restarted, or if the agent dynamically loads a new check containing
a secret handle (e.g. via Autodiscovery).

By design, the user-provided executable needs to implement any error handling
mechanism that a user might require. Conversely, the agent will need to be
restarted if a secret has to be refreshed in memory (e.g. revoked password).

This approach which relies on a user-provided executable has multiple benefits:

- Guarantees that the agent will not attempt to load in memory parameters for
  which there isn't a secret handle.
- Ability for the user to limit the visibility of the agent to secrets that
  it needs (e.g. by restraining in the key management backend the list of
  secrets that the executable can access).
- Maximum freedom and flexibility in allowing users to use any secrets
  management backend (including open source solutions such as `Vault` as well as
  closed sources ones) without having to rebuild the agent.
- Enabling each user to solve the **initial trust** problem from the agent to
  their secrets management backend, in a way that leverages their preferred
  authentication method, and fits into their continuous integration workflow.

The following are sample workflows documented by users. They are provided
for illustrative purposes, and not as leading practices. Each user should
define a workflow that fits their requirements and environment.

User A manages secrets centrally in a KMS, such as `Hashicorp Vault`. They store
the secret’s path and name as the handle (e.g. `mysql/prod/agent-a`), then
provide an executable that establishes trust with the KMS and makes web service
calls to it in order to retrieve secrets needed by the agent. In this setup,
User A was careful to appropriately configure the KMS so that the executable
only has read-only access, and only to secrets that the Datadog agent requires.

User B does not wish to provide access to a centralized KMS at run-time. They
store an encrypted version of the secret in the configuration file, then provide
an executable that can access an encryption key to decrypt it. In User B's
setup, the continuous integration pipeline generates a new symmetric encryption
key (e.g. in AWS KMS) for each new instance, uses it to encrypt secrets in the
Datadog configuration files by using a templating engine (e.g. consul template),
and ensures only the executable on this instance can access the encryption key.

Regardless of the workflow, the user should **take great care to secure the
executable itself**, including setting appropriate permissions and considering
the security implications of their executable in their environment.

### Configuration

In `datadog.yaml` you must set the following variables:

```yaml
secret_backend_command: /path/to/your/executable
```

More settings are available: see `datadog.yaml`.

### Agent security requirements

The agent will run `secret_backend_command` executable as a sub-process.

#### Linux

On Linux, the executable set as `secret_backend_command` **MUST** (the agent
will refuse to use it otherwise):

- Belong to the same user running the agent (by default `dd-agent` or `root`
  inside a container).
- Have **no** rights for `group` or `other`.
- Have at least `exec` right for the owner.

Also:
- The executable will be run with an empty environment.
- Never output sensitive information on STDERR. If the binary exit with a
  different status code than `0` the agent will log the standard error output
  of the executable to ease troubleshooting.

### The executable API

The executable has to respect a very simple API: it reads a JSON on the
Standard input and output a JSON containing the decrypted secrets on the
Standard output.

If the exit code of the executable is different than 0, the integration
configuration currently being decrypted will be considered erroneous and
dropped.

**Input:**

The executable will receive a JSON payload on the `Standard input` containing
the list of secrets to fetch:

```json
{
  "version": "1.0",
  "secrets": ["secret1", "secret2"]
}
```

- `version`: is a string containing the format version (currently "1.0").
- `secrets`: is a list of strings, each string is a **handle** from a
  configuration corresponding to a secret to fetch.

**Output:**

The executable is expected to output on the `Standard output` a JSON containing
the fetched secrets:

```json
{
  "secret1": {
    "value": "secret_value",
    "error": null
  },
  "secret2": {
    "value": null,
    "error": "could not fetch the secret"
  }
}
```

The expected payload is a JSON object, each key is one of the **handle**
requested in the input payload. The value for each **handle** is a JSON object
with 2 fields:

- `value`: a string: the actual secret value to be used in the check
  configurations (can be `null` in case of error).
- `error`: a string: the error message if needed. If `error` is different that
  `null` the integration configuration that uses this handle will be considered
  erroneous and dropped.

Example:

Here is a dummy Go program prefixing every secret with `decrypted_`:

```golang
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type secretsPayload struct {
	Secrets []string `json:secrets`
	Version int      `json:version`
}

func main() {
	data, err := ioutil.ReadAll(os.Stdin)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read from stdin: %s", err)
		os.Exit(1)
	}
	secrets := secretsPayload{}
	json.Unmarshal(data, &secrets)

	res := map[string]map[string]string{}
	for _, handle := range secrets.Secrets {
		res[handle] = map[string]string{
			"value": "decrypted_" + handle,
		}
	}

	output, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not serialize res: %s", err)
		os.Exit(1)
	}
	fmt.Printf(string(output))
}
```

This will update this configuration (in the check file):

```yaml
instances:
  - server: db_prod
    user: ENC[db_prod_user]
    password: ENC[db_prod_password]
```

to this (in the agent's memory):

```yaml
instances:
  - server: db_prod
    user: decrypted_db_prod_user
    password: decrypted_db_prod_password
```

## Troubleshooting

### Seeing configurations after secrets were injected

To quickly see how the check's configurations are resolved you can use the
`configcheck` command :

```shell
sudo -u dd-agent -- datadog-agent configcheck

=== a check ===
Source: File Configuration Provider
Instance 1:
host: <decrypted_host>
port: <decrypted_port>
password: <decrypted_password>
~
===

=== another check ===
Source: File Configuration Provider
Instance 1:
host: <decrypted_host2>
port: <decrypted_port2>
password: <decrypted_password2>
~
===
```

Note that the agent needs to be restarted to pick up changes on configuration files.

### Debugging your secret_backend_command

To test or debug outside of the agent you can simply mimic how the agent will run it:

#### Linux

```bash
sudo su dd-agent - bash -c "echo '{\"version\": \"1.0\", \"secrets\": [\"secret1\", \"secret2\"]}' | /path/to/the/secret_backend_command
```

The `dd-agent` user is created when you install the datadog-agent.

### Agent refusing to start

The first thing the agent does on startup is to load `datadog.yaml` and decrypt
any secrets in it. This is done before setting up the logging. This means that
on platform/setup errors occuring when loading `datadog.yaml` aren't written in
the logs but on stderr (this can occurs when the executable given to the agent
for secrets returns an error).

If you have secrets in `datadog.yaml` and the agent refuse to start: either try
to start the agent manually to be able to see STDERR or remove secrets from
`datadog.yaml` and test with secrets in a check configuration file first.
