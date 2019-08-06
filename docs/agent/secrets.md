# Secrets Management

Starting with version `6.3.0` on Linux and `6.11` on Windows, the Agent is able
to leverage the `secrets` package in order to call a user-provided executable
to handle retrieval or decryption of secrets, which are then loaded in memory
by the Agent. This feature allows users to no longer store passwords and other
secrets in plain text in configuration files. Users have the flexibility to
design their executable according to their preferred key management service,
authentication method, and continuous integration workflow.

Starting with version `6.11`, the Secrets Management feature is also supported
in the `datadog.yaml` file by APM (on Linux and Windows) and Process Monitoring
(on Linux only).

Starting with version `6.12` the Secrets Management feature is no longer in beta.

This section covers how to set up this feature.

- [Secrets Management](#secrets-management)
  - [Defining secrets in configurations](#defining-secrets-in-configurations)
  - [Retrieving secrets from the secret backend](#retrieving-secrets-from-the-secret-backend)
    - [Configuration](#configuration)
    - [Agent security requirements](#agent-security-requirements)
      - [Linux](#linux)
      - [Windows](#windows)
    - [The executable API](#the-executable-api)
  - [Troubleshooting](#troubleshooting)
    - [Listing detected secrets](#listing-detected-secrets)
    - [Seeing configurations after secrets were injected](#seeing-configurations-after-secrets-were-injected)
    - [Debugging your secret_backend_command](#debugging-your-secretbackendcommand)
      - [Linux](#linux-1)
      - [Windows](#windows-1)
        - [Rights related errors](#rights-related-errors)
        - [Testing your executable](#testing-your-executable)
    - [Agent refusing to start](#agent-refusing-to-start)

## Defining secrets in configurations

To declare a secret in a check configuration simply use the `ENC[]` notation.
This notation can be used to denote a secret as the *value* of any YAML field
in your configuration (not the key), in any section (`init_config`, `instances`,
`logs`, ...).

Secrets are supported in every configuration backend: file, etcd, consul ...

Starting version `6.10.0`, secrets are supported in environment variables.

Secrets are also supported in `datadog.yaml`. The Agent first loads the
main configuration and reloads it after decrypting the secrets. This means the
only place where secrets can't be used is the `secret_*` settings (see
Configuration section).

Secrets are always strings, this means you can't use this feature to set the
value of a setting of type integer or boolean (for example, `GUI_port`).

Example:

```yaml
instances:
  - server: db_prod
    # two valid secret handles
    user: "ENC[db_prod_user]"
    password: "ENC[db_prod_password]"

    # The `ENC[]` handle must be the entire YAML value, which means that
    # the following is NOT detected as a secret handle:
    password2: "db-ENC[prod_password]"
```

In the above example there are two secrets : `db_prod_user` and
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

There is no need to escape inner `[` and `]`. The Agent selects everything between the first `ENC[` and the last `]`.

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

The Agent caches secrets internally in memory to reduce the number of calls
(useful in a containerized environment for example). The Agent calls the
executable every time it accesses a check configuration file that contains at
least one secret handle for which the secret is not already loaded in memory. In
particular, secrets that have already been loaded in memory do not trigger
additional calls to the executable. In practice, this means that the Agent calls
the user-provided executable once per file that contains a secret handle at
startup, and might make additional calls to the executable later if the Agent or
instance is restarted, or if the Agent dynamically loads a new check containing
a secret handle (e.g. via Autodiscovery).

Since APM and Process Monitoring run in their own process/service, and since
processes don't share memory, each needs to be able to load/decrypt secrets.
Thus, if `datadog.yaml` contains secrets, each process might call the executable
once. For example, storing the `api_key` as a secret in the `datadog.yaml` file
with APM and Process Monitoring enabled might result in 3 calls to the secret backend.

By design, the user-provided executable needs to implement any error handling
mechanism that a user might require. Conversely, the Agent needs to be
restarted if a secret has to be refreshed in memory (e.g. revoked password).

This approach which relies on a user-provided executable has multiple benefits:

- Guarantees that the Agent does not attempt to load in memory parameters for
  which there isn't a secret handle.
- Ability for the user to limit the visibility of the Agent to secrets that
  it needs (e.g. by restraining in the key management backend the list of
  secrets that the executable can access).
- Maximum freedom and flexibility in allowing users to use any secrets
  management backend (including open source solutions such as `Vault` as well as
  closed sources ones) without having to rebuild the Agent.
- Enabling each user to solve the **initial trust** problem from the Agent to
  their secrets management backend, in a way that leverages their preferred
  authentication method, and fits into their continuous integration workflow.

The following are sample workflows documented by users. They are provided
for illustrative purposes, and not as leading practices. Each user should
define a workflow that fits their requirements and environment.

User A manages secrets centrally in a KMS, such as `Hashicorp Vault`. They store
the secret’s path and name as the handle (e.g. `mysql/prod/agent-a`), then
provide an executable that establishes trust with the KMS and makes web service
calls to it in order to retrieve secrets needed by the Agent. In this setup,
User A was careful to appropriately configure the KMS so that the executable
only has read-only access, and only to secrets that the Datadog Agent requires.

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

The Agent runs `secret_backend_command` executable as a sub-process. The
execution patterns differ on Linux and Windows.

#### Linux

On Linux, the executable set as `secret_backend_command` **MUST** (the Agent
refuses to use it otherwise):

- Belong to the same user running the Agent (by default `dd-agent` or `root`
  inside a container).
- Have **no** rights for `group` or `other`.
- Have at least `exec` right for the owner.

Also:
- The executable shares the same environment variables as the Agent.
- Never output sensitive information on STDERR. If the binary exits with a
  different status code than `0` the Agent logs the standard error output
  of the executable to ease troubleshooting.

#### Windows

On Windows, the executable set as `secret_backend_command` **MUST** (the Agent
refuses to use it otherwise):

- Have `Read/Exec` for `ddagentuser` (the user used to run the Agent).
- Have **no** rights for any user or group except `Administrator` or `LocalSystem`.
- Be a valid Win32 application so the Agent can execute it.

Also:
- The executable shares the same environment variables as the Agent.
- Never output sensitive information on STDERR. If the binary exit with a
  different status code than `0`, the Agent logs the standard error output
  of the executable to ease troubleshooting.

Here is an example of a [powershell script](secrets_scripts/set_rights.ps1)
that removes rights on a file to everybody except from `Administrator` and
`LocalSystem` and then add `ddagentuser`. Use it only as an example as your
setup might differ.

The Agent GUI does **NOT** show the decrypted password but instead shows the raw handle. This
is on purposes as it lets you edit configuration files. To see the final config
after being decrypted use the `configcheck` command on the datadog-agent CLI.

### The executable API

The executable has to respect a very simple API: it reads a JSON on the
Standard input and output a JSON containing the decrypted secrets on the
Standard output.

If the exit code of the executable is different than 0, the integration
configuration currently being decrypted will be considered erroneous and
dropped.

**Input:**

The executable receives a JSON payload on the `Standard input` containing
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

### Listing detected secrets

The `secret` command in the Agent CLI shows any errors related to your setup
(if the rights on the executable aren't the right one for example). It
also lists all handles found and where they where found.

On Linux the command outputs file mode, owner and group for the executable,
on Windows ACL rights are listed.

Example on Linux:

```shell
$> datadog-agent secret
=== Checking executable rights ===
Executable path: /path/to/you/executable
Check Rights: OK, the executable has the correct rights

Rights Detail:
file mode: 100700
Owner username: dd-agent
Group name: dd-agent

=== Secrets stats ===
Number of secrets decrypted: 3
Secrets handle decrypted:
- api_key: from datadog.yaml
- db_prod_user: from postgres.yaml
- db_prod_password: from postgres.yaml
```

Example on Windows (from an Administrator Powershell):
```powershell
PS C:\> & 'C:\Program Files\Datadog\Datadog Agent\embedded\agent.exe' secret
=== Checking executable rights ===
Executable path: C:\path\to\you\executable.exe
Check Rights: OK, the executable has the correct rights

Rights Detail:
Acl list:
stdout:


Path   : Microsoft.PowerShell.Core\FileSystem::C:\path\to\you\executable.exe
Owner  : BUILTIN\Administrators
Group  : WIN-ITODMBAT8RG\None
Access : NT AUTHORITY\SYSTEM Allow  FullControl
         BUILTIN\Administrators Allow  FullControl
         WIN-ITODMBAT8RG\ddagentuser Allow  ReadAndExecute, Synchronize
Audit  :
Sddl   : O:BAG:S-1-5-21-2685101404-2783901971-939297808-513D:PAI(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200
         a9;;;S-1-5-21-2685101404-2783901971-939297808-1001)

=== Secrets stats ===
Number of secrets decrypted: 3
Secrets handle decrypted:
- api_key: from datadog.yaml
- db_prod_user: from sqlserver.yaml
- db_prod_password: from sqlserver.yaml
```

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

**Note**:  The Agent needs to be restarted to pick up changes on configuration files.

### Debugging your secret_backend_command

To test or debug outside of the Agent you can mimic how the Agent runs it:

#### Linux

```bash
sudo su dd-agent - bash -c "echo '{\"version\": \"1.0\", \"secrets\": [\"secret1\", \"secret2\"]}'" | /path/to/the/secret_backend_command
```

The `dd-agent` user is created when you install the datadog-agent.

#### Windows

##### Rights related errors

If you encounter one of the following errors then something is missing in your
setup. See the [Windows intructions](#windows).

1. If any other group or user than needed has rights on the executable a similar error is written to the log:
   ```
   error while decrypting secrets in an instance: Invalid executable 'C:\decrypt.exe': other users/groups than LOCAL_SYSTEM, Administrators or ddagentuser have rights on it
   ```

2. If `ddagentuser` doesn't have read and execute right on the file, a similar error is written to the log:
   ```
   error while decrypting secrets in an instance: could not query ACLs for C:\decrypt.exe
   ```

3. Your executable needs to be a valid Win32 application, the following error is written to the log:
   ```
   error while running 'C:\decrypt.py': fork/exec C:\decrypt.py: %1 is not a valid Win32 application.
   ```

##### Testing your executable

Your executable is executed by the Agent when fetching your secrets. The
Datadog Agent runs using the `ddagentuser`. This user has no specific
rights but is part of the `Performance Monitor Users` group. The password for
this user is randomly generated at install time and is never saved anywhere.

This means that your executable might work with your default user or
development user but not when it's run by the Agent, as `ddagentuser` has more
restricted rights.

The easiest way to test your executable in the same conditions as the Agent is
to update the password of the `ddagentuser` on your dev box. This way you can
authenticate as `ddagentuser` and run your executable in the same context the Agent would.

To do so, follow those steps:
1. Remove `ddagentuser` from the `Local Policies/User Rights Assignement/Deny
   Log on locally` list in the `Local Security Policy`.
2. Set a new password for ddagentuser (since the one generated at install time
   is never saved anywhere). In Powershell run:
   ```powershell
   $user = [ADSI]"WinNT://./ddagentuser";
   $user.SetPassword("a_new_password")
   ```
3. Update the password to be used by `DatadogAgent` service in the Service
   Control Manager. In powershell run:
   ```powershell
   sc.exe config DatadogAgent password= "a_new_password"
   ```

You can now login as `ddagentuser` to test your executable. We have a
[powershell script](secrets_scripts/secrets_tester.ps1) to help you test your
executable as another user. It switches user context and mimics how the
Agent runs your executable.

Example on how to use it:
```powershell
.\secrets_tester.ps1 -user ddagentuser -password a_new_password -executable C:\path\to\your\executable.exe -payload '{"version": "1.0", "secrets": ["secret_ID_1", "secret_ID_2"]}'
Creating new Process with C:\path\to\your\executable.exe
Waiting a second for the process to be up and running
Writing the payload to Stdin
Waiting a second so the process can fetch the secrets
stdout:
{"secret_ID_1":{"value":"secret1"},"secret_ID_2":{"value":"secret2"}}
stderr: None
exit code:
0
```

### Agent refusing to start

The first thing the Agent does on startup is to load `datadog.yaml` and decrypt
any secrets in it. This is done before setting up the logging. This means that
on platforms like Windows, errors occurring when loading `datadog.yaml` aren't
written in the logs but on stderr (this can occur when the executable given to
the Agent for secrets returns an error).

If you have secrets in `datadog.yaml` and the Agent refuses to start: either try
to start the Agent manually to be able to see STDERR or remove the secrets from
`datadog.yaml` and test with secrets in a check configuration file first.

