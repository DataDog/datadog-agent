# One-time E2E setup

`/docs/public/how-to/test/e2e.md` is the authoritative guide to `dda inv -- e2e.setup`. This file adds
only which machine to run it on. It is idempotent, so suggesting it is cheap even when you are unsure.

**Always on the host, never the interactive form in a container.** It derives the keypair name from the
OS username, which is `dd` in every container, so an interactive run there mints a second keypair in the
shared account. Worse, it then only works once: the task hard-fails when a keypair exists in AWS but not
on disk, which is exactly what the next container sees, and the only way out is deleting the keypair by
hand. `--no-interactive` is the safe in-container form — it does the Pulumi work and skips the AWS and
config-file work entirely.

Note one host mutation: the run task's preflight appends the SSO profile to `~/.aws/config` when absent,
and `~/.aws` is bind-mounted read-write into a dev env, so a run inside a container can write to the
host's file. It is idempotent and skips when the profile is already there.
