# Secrets 

## Configuring

You need to specify the path of the decrypting program by specifying `secret_backend_command`
command.

All secrets should follow pattern "ENC[keyname]" 

```
secret_backend_command: /etc/stackstate-agent/dummy_secret_feeder.sh

# The StackState api key to associate your Agent's data with your organization.
api_key: "ENC[api_key]"

```

stackstate-agent will pass the following json request to stdin of the decrypting program,
where `secrets` contains the array of the secrets to be returned

```
{
  "version": "1.0",
  "secrets": ["keyname", "api_key"]
}
```

stackstate-agent expects that the decrypting program will return the json output with
decoded secrets

```
{
  "api_key": {
    "value": "YOUR_API_KEY",
    "error": null
  },
  "keyname": {
    "value": null,
    "error": "could not fetch the secret"
  }
}
```

## Troubleshouting

The decrypting program should be exclusively owned by user stackstate-agent runs under,
in other case you will get error like

```
config.load unable to decrypt secret from stackstate.yaml: invalid executable: '/etc/stackstate-agent/dummy_secret_feeder.sh' isn't owned by the XXX running the agent: name 'YYY', UID ZZZ. We can't execute it
```

You can take a look which secrets are currently picked
by configuration using `stackstate-agent secret` command

```
 sudo -u stackstate-agent stackstate-agent secret
=== Checking executable rights ===
Executable path: /etc/stackstate-agent/dummy_secret_feeder.sh
Check Rights: OK, the executable has the correct rights

Rights Detail:
file mode: 100700
Owner username: stackstate-agent
Group name: stackstate-agent

=== Secrets stats ===
Number of secrets decrypted: 1
Secrets handle decrypted:
- api_key: from stackstate.yaml

```

You can debug your decoding script from console using the approach below:

```
sudo su stackstate-agent - bash -c "echo '{\"version\": \"1.0\", \"secrets\": [\"secret1\", \"secret2\"]}' | /path/to/the/secret_backend_command"
```

You can show decrypted config with:

    sudo -u stackstate-agent -- stackstate-agent configcheck
