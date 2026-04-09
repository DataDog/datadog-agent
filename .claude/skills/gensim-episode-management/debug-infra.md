# Debugging gensim-eks Infrastructure

Troubleshooting guide for Pulumi, helm, kubectl, and cluster issues when running
gensim episodes on EKS.

## Diagnostic Flowchart

Start here and follow the branch that matches the symptom:

1. **Submit command fails** -> "Submit Failures" section
2. **Run stuck in episode-install** -> "Orchestrator Failures" section
3. **kubectl/pup commands fail** -> "Auth & Connectivity" section
4. **Run completes but no events** -> "Detection Issues" section

## Submit Failures

### "Cluster already exists" (409 ResourceInUseException)

Pulumi state lost track of the EKS cluster (usually from a cancelled `pulumi refresh`).
Pulumi tries to CREATE it but AWS says it exists.

**Fix**: Refresh state to rediscover the existing cluster:
```bash
cd test/e2e-framework/run
pulumi refresh --stack <stack-name> --yes
```

If refresh doesn't help (the `aws:eks:Cluster` child resource was dropped), the
nuclear option is destroy + recreate:
```bash
# 1. Delete the orphaned cluster from AWS
aws-vault exec sso-agent-sandbox-account-admin -- \
  aws eks delete-cluster --name <stack-name> --region us-east-1

# 2. Wait for deletion (~3-5 min)
watch -n 15 "aws-vault exec sso-agent-sandbox-account-admin -- \
  aws eks describe-cluster --name <stack-name> --region us-east-1 \
  --query 'cluster.status' --output text 2>&1"

# 3. Destroy Pulumi state
cd test/e2e-framework/run
PULUMI_K8S_DELETE_UNREACHABLE=true pulumi destroy --stack <stack-name> --yes

# 4. Resubmit (will create everything fresh, ~15 min)
```

### Stack lock

Previous Pulumi run was interrupted or killed:
```bash
pulumi cancel --stack <stack-name> --yes
```

### "Scenario file not found"

Wrong scenario name. List available scenarios:
```bash
ls gensim-episodes/*/<episode>/episodes/
```

### Build VM SSH timeout during refresh

Pulumi refresh hangs trying to SSH into remote command resources on the build VM.
Common after a weekend when the infra cleaner may have stopped instances.

**Diagnose**: Check if the build VM exists and is reachable:
```bash
# Check EC2
aws-vault exec sso-agent-sandbox-account-admin -- \
  aws ec2 describe-instances --filters "Name=tag:Name,Values=*gensim-builder*" \
  --query 'Reservations[].Instances[].{ID:InstanceId,State:State.Name,IP:PrivateIpAddress}' \
  --region us-east-1 --output table

# Test SSH connectivity (requires VPN/Appgate)
nc -z -w 5 <private-ip> 22
```

If the VM exists and is reachable but Pulumi still hangs, it may be a pipe issue --
don't pipe Pulumi output through `head` or `tail` as SIGPIPE kills the process.

### Security group "DependencyViolation" during destroy

The EKS cluster (which Pulumi doesn't know about) still references the security group.
Delete the EKS cluster from AWS first (see "Cluster already exists" section above),
then retry destroy.

## Orchestrator Failures

### "INSTALLATION FAILED: cannot re-use a name that is still in use"

Stale helm release from a previous failed run. Helm tracks releases via secrets:

```bash
# List helm release secrets
kubectl --context aws get secrets -n default -l owner=helm

# Delete them
kubectl --context aws delete secret -l owner=helm -n default

# Delete the failed orchestrator job
kubectl --context aws delete job gensim-orchestrator -n default

# Resubmit
```

### "INSTALLATION FAILED: context deadline exceeded"

Helm's `--wait` timed out. Usually NOT a resource issue. Check pod states:

```bash
kubectl --context aws get pods -n default -o wide
```

**ImagePullBackOff**: Pods are trying to pull from the wrong registry. If images
reference `docker.io/gensim/<service>:latest` instead of
`<ecr-registry>/gensim/<service>:latest`, the ECR registry wasn't passed to helm.
This happens when `--skip-build` is used with the old code that didn't compute
`imageRegistry`. The fix is in the `--skip-build` implementation (it now always
resolves ECR).

**Pending/Unschedulable**: Node is full. Check node resources:
```bash
kubectl --context aws describe node <node-name> | grep -A10 "Allocated resources"
```

### "invalid release name, must match regex... length must not be longer than 53"

Episode name is too long for a helm release. The orchestrator truncates to
`gensim-` + 46 chars. If you see this error, the truncation isn't working --
check `orchestrator.sh.tmpl` line 142 for the `cut -c1-46`.

### Orchestrator pod in Error state

```bash
kubectl --context aws logs -l job-name=gensim-orchestrator -n default --tail=30
```

The orchestrator is a Kubernetes Job with `restartPolicy: Never`. If it fails,
you must delete the job before resubmitting:
```bash
kubectl --context aws delete job gensim-orchestrator -n default
```

## Auth & Connectivity

### kubectl "Unauthorized" or "connection refused"

**Stale kubeconfig**: After a cluster recreate, refresh:
```bash
aws-vault exec sso-agent-sandbox-account-admin -- \
  aws eks update-kubeconfig --name <stack-name> --region us-east-1 \
  --kubeconfig <kubeconfig-path>
```

**Wrong context**: The kubeconfig may have two contexts. Pulumi creates an `aws`
context that works. The default context from `update-kubeconfig` may not:
```bash
kubectl --context aws get pods  # use this
```

**AWS STS expired**: kubectl shells out to `aws eks get-token` which needs valid
AWS credentials:
```bash
aws-vault exec sso-agent-sandbox-account-admin -- aws sts get-caller-identity
```

### kubectl "no such host"

The EKS cluster DNS was cleaned up (infra cleaner). The cluster needs to be
recreated -- see "Submit Failures" section.

### pup "authentication required"

```bash
pup auth login
```

This expires frequently (~30 min). Re-auth as needed.

## Detection Issues

### Zero events after disruption

Checklist:
1. **Is the episode actually in disruption?** Check run-status configmap phase.
2. **Is analysis enabled?** Check deployed agent config:
   ```bash
   kubectl --context aws get configmap gensim-agent-values -n default -o yaml | grep -A5 "analysis"
   ```
3. **Is event_reporter.sending_enabled true?** Same configmap check.
4. **Is min_cluster_size too high?** Most scenarios produce clusters of size 1-2.
   If set to 3+, all events get filtered.
5. **Have detectors completed warmup?** Detectors need a minimum number of data
   points before they start detecting. High-frequency metrics (e.g. virtual logs
   at ~1s) warm up fast; low-frequency metrics (e.g. check metrics at ~15s) may
   need longer than the baseline phase. See `references/current-detector-issues.md`.
6. **Check pup auth** -- maybe events are firing but your query is failing silently.

### Events fire but only on infrastructure metrics

The observer detects on ALL metrics flowing through it, including container,
coredns, and system metrics. Scenario-specific metrics (redis, trace, kafka) may
not fire if the detector hasn't completed warmup for those slower-interval metrics.
See `references/current-detector-issues.md` for current warmup requirements.

### Detector works in testbench but not live

Some detectors (e.g. batch/retrospective detectors like ScanWelch) may produce
anomalies in testbench replay but not emit Datadog events in live mode. This can
happen when the correlator's eviction window is shorter than the detector's
detection delay. See `references/current-detector-issues.md` for details and
check `engine.go` `runDetectorsAndCorrelatorsSnapshot()` for the current
execution sequence.

## Quick Recovery Playbook

When everything is broken and you need a clean slate:

```bash
# 1. Cancel any Pulumi lock
cd test/e2e-framework/run
pulumi cancel --stack <stack-name> --yes 2>/dev/null

# 2. Delete orphaned EKS cluster if it exists
aws-vault exec sso-agent-sandbox-account-admin -- \
  aws eks delete-cluster --name <stack-name> --region us-east-1 2>/dev/null

# 3. Wait for deletion
sleep 180  # or poll with aws eks describe-cluster

# 4. Destroy Pulumi state
PULUMI_K8S_DELETE_UNREACHABLE=true pulumi destroy --stack <stack-name> --yes

# 5. Fresh submit
cd <agent-repo>
dda inv aws.eks.gensim.submit --image=<tag> --episodes=<ep:scen> --mode=live-and-record --s3-bucket=qbranch-gensim-recordings
```

Total time: ~20 min for clean-slate recovery.
