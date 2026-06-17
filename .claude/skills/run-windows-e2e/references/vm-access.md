# Accessing the test VM — Windows E2E Tests

When `devMode` is enabled (see [setup.md](setup.md)) the VM persists after the
test, so you can RDP or SSH in to investigate.

## Find the stack name

Two ways:

- From test output: look for `Creating workspace for stack: user-e2e-<suitename>-<hash>`
- From the CLI: `pulumi stack ls --all` — find the entry matching the suite name

Format: `user-e2e-<suitename-lowercase>-<hash>`.

## Get connection details

```bash
pulumi stack --stack organization/e2elocal/<stack-name> output --json --show-secrets
```

The output includes `address` (private IP), `username` (`Administrator`),
`password`, and an `osFamily` integer. The OS family/flavor enum values are in
`test/e2e-framework/components/os/const.go` (`WindowsFamily = 2`,
`WindowsServer = 509`, `WindowsClient = 510`).

## RDP

```bash
# Opens RDP, prints IP + password, copies password to clipboard
dda inv aws.rdp-vm --stack-name <stack-name>
```

`dda inv aws.show-vm` does **not** work for e2e test stacks — it is only for VMs
created with `dda inv aws.create-vm` (it prepends the username twice and looks
for the wrong output key, `"aws-vm"` instead of `"dd-Host-aws-vm"`).

## SSH

The SSH key is the `privateKeyPath` from `~/.test_infra_config.yaml`.

```bash
ssh -i <privateKeyPath> -o StrictHostKeyChecking=no <username>@<ip> "<commands>"
```

Pick the command separator from `osFamily` in the stack output:

- `osFamily: 1` = Linux → `&&` or `;` (bash)
- `osFamily: 2` = Windows → `;` only (PowerShell over SSH; `&&` is invalid)

### SSH availability

SSH should be reachable within ~60s for Linux VMs and ~180s for Windows VMs
after the VM is provisioned. If it takes longer:

- Make sure the key is loaded in your SSH agent (`ssh-add -l`) or pass `-i`
  explicitly.
- Verify the keypair name in `~/.test_infra_config.yaml` matches the AWS key
  pair used to provision the VM — a mismatch means the wrong public key was
  injected and authentication will always fail.
