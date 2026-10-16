# Optional Ubuntu bootstrap

Read this only when `vm.sh probe` reports missing tools, Docker permission failures, missing PyYAML, or low inotify
limits. A pre-provisioned VM is preferable; building or changing an AMI is outside this skill's scope.

`scripts/bootstrap.sh` installs missing Docker CE, kubectl 1.32, kind 0.27.0 and Helm 3.20.2, plus PyYAML, and configures
inotify limits. It is restricted to disposable Ubuntu x86_64 hosts. Review its code before adapting another Linux image.
The versions come from `q_branch/gadget-k8s-host.lima.yaml`.

From the local machine:

```bash
bash "$SKILL/scripts/vm.sh" bootstrap --disposable-vm "$VM"
```

The helper copies the installer to `~/gensim/bootstrap.sh`, runs it, and closes an existing SSH multiplexed connection.
Open a new session so Docker group membership takes effect, then rerun `probe`. Stop on a failed installation rather
than proceeding with partially installed tooling. The installer adds Python 3 and PyYAML when needed.
