# [BETA] Encrypted secrets

**This feature is in beta and its options or behavior might break between
minor or bugfix releases of the Agent.**

This feature is only available on Linux at the moment.

Starting with version `6.3.0`, the agent is able to collect encrypted secrets
in the configurations from an external source. This allows users to use the
agent alongside a secret management service (like `Vault` or other).

This section will cover how to set up this feature.

## Encrypting password in check configurations

To fetch a secret from a check configuration simply use the `ENC[]` notation.
This handle can be used as a *value* of any YAML fields in your configuration
(not the key) in any section (`init_config`, `instances`, `logs`, ...).

Encrypted secrets are supported in every configuration backend: file, etcd,
consul ...

Example:

```yaml
instances:
  - server: db_prod
    # two valid handles
    user: ENC[db_prod_user]
    password: "ENC[db_prod_password]"

    # The `ENC[]` handle must be the entire YAML value. Which means the
    # following handle won't be detected
    password2: "some test ENC[db_prod_password]"
```

In the above example we have two encrypted  secrets : `db_prod_user` and
`db_prod_password`. Those are the secrets **handles** and must uniquely identify
a secret within your secrets management tool.

Between the brackets every character is allowed as long as the YAML configuration
is valid. This means you could use any format you want.

Example:

```
ENC[{"env": "prod", "check": "postgres", "id": "user_password", "az": "us-east-1a"}]
```

In this example the secret handle is the string `{"env": "prod", "check":
"postgres", "id": "user_password", "az": "us-east-1a"}`.

**Autodiscovery**:

Secrets are resolved **after** Autodiscovery template variables. This means you
can use them in a secret handle to fetch secrets specific to a container.

Example:

```
instances:
  - server: %%host%%
    user: ENC[db_prod_user_%%host%%]
    password: ENC[db_prod_password_%%host%%]
```

## Fetching secrets

To fetch the secrets from your configurations you have to provide an executable
capable of fetching passwords from your secrets management tool.

An external executable solved multiple issues:

- Guarantees that the agent will never have more access/information than what
  you transfer to it.
- Maximum freedom and flexibility: allowing users to use any secrets management
  tool (including closed sources ones) without having to rebuild the agent.
- Solving the **initial trust** is a very complex problem as it usually changes
  from users to users. Each setup requires more or less control and might rely on
  private tools. This shifts the problem to where it is easier to solve, without
  having to rebuild the agent.

This executable will be executed by the agent once per check's instance. The
agent will cache secrets internally to reduce the number of calls (useful in
a containerized environment for example).

The executable **MUST** (the agent will refuse to use it otherwise):

- Belong to the same user running the agent (usually `dd-agent`).
- Have **no** rights for `group` or `other`.
- Have at least `exec` right for the owner.
- The executable will not share any environment variables with the agent.

### Configuration

In `datadog.yaml` you must set the following variables:

```yaml
secret_backend_command: /path/to/your/executable
```

More settings are available: see `datadog.yaml`.

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

The expected payload is a JSON object, each key is one of the **handle** requested in the input payload.
The value for each **handle** is a JSON object with 2 fields:

- `value`: a string: the actual secret value to be used in the check
  configurations (can be `null` in case of error).
- `error`: a string: the error message if needed. If `error` is different that
  `null` the integration configuration that uses this handle will be considered
  erroneous and dropped.

Example:

Here is a dummy script prefixing every secret by `decrypted_`:

```py
#!/usr/bin/python

import json
import sys

# Reading the input payload from STDIN
payload = sys.stdin.read()
# parsing the payload
requested_secrets = json.loads(payload)

secrets = {}
for secret_handle in requested_secrets["secrets"]:
    secrets[secret_handle] = {"value": "decrypted_"+secret_handle, "error": None}

print json.dumps(secrets)
```

This will update this configuration:

```yaml
instances:
  - server: db_prod
    user: ENC[db_prod_user]
    password: ENC[db_prod_password]
```

to this:

```yaml
instances:
  - server: db_prod
    user: decrypted_db_prod_user
    password: decrypted_db_prod_password
```

