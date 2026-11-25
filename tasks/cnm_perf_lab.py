"""
Run the CNM agent performance lab on its driver instance in agent-sandbox, from CI.

The lab (ddoghq/system-probe-test-environments, k8s/cnm-perf-lab) creates an EKS cluster, runs fixed-rate network load
against agent variants, measures system-probe CPU and eBPF cost, and destroys the cluster. CI runners can't reach the
cluster's private API endpoint, so the lab runs on a driver instance inside the VPC; these tasks only start the driver,
hand it the run over SSM, follow it, and collect the results. The driver destroys the cluster and powers itself off when
the run ends, whatever the outcome.
"""

from __future__ import annotations

import json
import math
import os
import shlex
import time
from pathlib import Path

from invoke import Context, Exit, task

REGION = "us-east-1"
ACCOUNT = "376334461865"  # agent-sandbox
CI_ROLE = f"arn:aws:iam::{ACCOUNT}:role/ddbuild-cnm-perf-lab-ci"
CI_ROLE_EXTERNAL_ID = "ddbuild-cnm-perf-lab-ci"
DRIVER_TAG = "cnm-perf-lab-driver"  # tag:service on the driver instance
BUCKET = "datadog-cnm-perf-lab"
LAB_TARBALL_KEY = "inputs/cnm-perf-lab.tgz"
# The driver reads the agents' staging API key from this secret (JSON with api_key) when its run starts. With
# CNM_PERF_LAB_STAGING_API_KEY set (dd-sts's shared org 2 staging key, from the CI job), the run fills the secret and
# blanks it when the run ends; KEY_WRITTEN records that, so the cancel task can blank it too.
LAB_KEYS_SECRET = "cnm-perf-lab/staging-keys"
KEY_WRITTEN = Path("cnm-perf-lab-key-written")
RUN_COMMENT = "cnm-perf-lab run"  # prefix of the SSM command comment, to find a run in progress
MARKER = Path("cnm-perf-lab-command-id")  # "<instance id> [<command id>]", for the cancel task
# SSM kills the command at this timeout with SIGKILL, skipping driver-run.sh's teardown. The driver gets a deadline
# DEADLINE_MARGIN_S before it and stops its steps early enough to tear down by then.
EXECUTION_TIMEOUT_S = 6 * 3600
DEADLINE_MARGIN_S = 10 * 60
# Chained role sessions last at most 1 h, and a full run takes about 3 h: re-assume well before expiry
SESSION_REFRESH_S = 45 * 60
METRIC_PREFIX = "cnm_perf_lab"
# Default variant slots for --variants=release: the latest 7.x GA, a fixed anchor (catches regressions that already
# shipped), and main. Each resolves to a digest when the run starts. CNM_PERF_LAB_REF_<A|B|C> overrides a slot.
ANCHOR_VERSION = "7.82.1"
RELEASE_SLOTS = {
    "A": ("ga", None),
    "B": ("anchor", f"datadog/agent:{ANCHOR_VERSION}"),
    "C": ("main", "datadog/agent-dev:nightly-main-py3"),
}


class _Aws:
    """Clients for the CI role, re-assumed every SESSION_REFRESH_S seconds."""

    def __init__(self):
        self._session = None
        self._assumed_at = 0.0

    def client(self, name: str):
        import boto3

        if self._session is None or time.time() - self._assumed_at > SESSION_REFRESH_S:
            creds = boto3.client("sts", region_name=REGION).assume_role(
                RoleArn=CI_ROLE,
                ExternalId=CI_ROLE_EXTERNAL_ID,
                RoleSessionName=f"cnm-perf-lab-{os.environ.get('CI_JOB_ID', 'local')}",
            )["Credentials"]
            self._session = boto3.Session(
                aws_access_key_id=creds["AccessKeyId"],
                aws_secret_access_key=creds["SecretAccessKey"],
                aws_session_token=creds["SessionToken"],
                region_name=REGION,
            )
            self._assumed_at = time.time()
        return self._session.client(name)


def _clear_staging_key(aws: _Aws) -> None:
    if KEY_WRITTEN.exists():
        aws.client("secretsmanager").put_secret_value(
            SecretId=LAB_KEYS_SECRET, SecretString=json.dumps({"api_key": ""})
        )
        KEY_WRITTEN.unlink()
        print(f"blanked {LAB_KEYS_SECRET}")


def _hub_get(url: str, token: str | None = None, method: str = "GET", accept: str | None = None):
    import urllib.request

    # Docker Hub refuses urllib's default user agent
    req = urllib.request.Request(url, method=method, headers={"User-Agent": "datadog-agent-ci/cnm-perf-lab"})
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if accept:
        req.add_header("Accept", accept)
    return urllib.request.urlopen(req, timeout=30)


def _latest_ga() -> str:
    """The highest 7.x.y tag of datadog/agent on Docker Hub, among the most recently pushed tags."""
    import re

    # Newest first; anonymous requests can't page past 1000 tags, and a new GA is always among the recent ones
    url = "https://hub.docker.com/v2/repositories/datadog/agent/tags?page_size=100&name=7.&ordering=last_updated"
    versions = []
    for _ in range(5):
        page = json.load(_hub_get(url))
        versions += [t["name"] for t in page["results"] if re.fullmatch(r"7\.\d+\.\d+", t["name"])]
        url = page.get("next")
        if not url:
            break
    if not versions:
        raise Exit("no 7.x.y tag found for datadog/agent on Docker Hub", code=1)
    return max(versions, key=lambda v: tuple(int(x) for x in v.split(".")))


def _pin(ref: str) -> str:
    """Resolve a Docker Hub image tag to repo@digest, so a run measures exactly one image. Other registries pass through."""
    if "@sha256:" in ref:
        return ref
    first, _, rest = ref.partition("/")
    if rest and ("." in first or ":" in first or first == "localhost"):
        if first not in ("docker.io", "registry-1.docker.io", "index.docker.io"):
            print(f"warning: {ref} is not on Docker Hub; using it unpinned")
            return ref
        ref = rest
    repo, sep, tag = ref.rpartition(":")
    if not sep or "/" in tag:
        repo, tag = ref, "latest"
    if "/" not in repo:
        repo = f"library/{repo}"
    scope = f"repository:{repo}:pull"
    token = json.load(_hub_get(f"https://auth.docker.io/token?service=registry.docker.io&scope={scope}"))["token"]
    accept = ", ".join(
        [
            "application/vnd.oci.image.index.v1+json",
            "application/vnd.docker.distribution.manifest.list.v2+json",
            "application/vnd.docker.distribution.manifest.v2+json",
        ]
    )
    resp = _hub_get(f"https://registry-1.docker.io/v2/{repo}/manifests/{tag}", token, "HEAD", accept)
    return f"{repo}@{resp.headers['Docker-Content-Digest']}"


def _release_refs() -> dict[str, tuple[str, str]]:
    """Variant -> (role, pinned ref) for --variants=release."""
    refs = {}
    for variant, (role, default) in RELEASE_SLOTS.items():
        ref = os.environ.get(f"CNM_PERF_LAB_REF_{variant}") or default or f"datadog/agent:{_latest_ga()}"
        refs[variant] = (role, _pin(ref))
        print(f"variant {variant} ({role}): {ref} -> {refs[variant][1]}")
    return refs


def _driver_id(aws: _Aws) -> str:
    reservations = aws.client("ec2").describe_instances(
        Filters=[
            {"Name": "tag:service", "Values": [DRIVER_TAG]},
            {"Name": "instance-state-name", "Values": ["pending", "running", "stopping", "stopped"]},
        ]
    )["Reservations"]
    ids = [i["InstanceId"] for r in reservations for i in r["Instances"]]
    if len(ids) != 1:
        raise Exit(f"expected exactly one instance tagged service={DRIVER_TAG}, found {ids}", code=1)
    return ids[0]


def _start_driver(aws: _Aws, instance_id: str) -> None:
    ec2 = aws.client("ec2")
    state = ec2.describe_instances(InstanceIds=[instance_id])["Reservations"][0]["Instances"][0]["State"]["Name"]
    if state == "stopping":
        ec2.get_waiter("instance_stopped").wait(InstanceIds=[instance_id])
        state = "stopped"
    if state == "stopped":
        print(f"starting the driver {instance_id}")
        ec2.start_instances(InstanceIds=[instance_id])
    ec2.get_waiter("instance_running").wait(InstanceIds=[instance_id])
    deadline = time.time() + 600
    while time.time() < deadline:
        info = aws.client("ssm").describe_instance_information(
            Filters=[{"Key": "InstanceIds", "Values": [instance_id]}]
        )["InstanceInformationList"]
        if info and info[0]["PingStatus"] == "Online":
            return
        time.sleep(10)
    raise Exit(f"the driver {instance_id} never registered with SSM", code=1)


def _driver_command(run_id: str, mode: str, refs_key: str | None, check_ndq: bool = True) -> str:
    """The shell script the driver runs: unpack the lab, set the driver up, run it, and power off afterwards."""
    lab = "/opt/cnm-perf-lab-dist"
    refs = ""
    if refs_key:
        refs = (
            f"mkdir -p {lab}/refs && aws s3 cp --only-show-errors s3://{BUCKET}/{refs_key} {lab}/refs/REFS.txt\n"
            f"export LAB_IMAGES_OVERRIDE={lab}/refs\n"
        )
    # Results are in S3 by now, so drop earlier runs' copies (driver-run.sh resets the rest of LAB_WORK itself).
    # A previous run's driver-run.sh schedules a poweroff a minute out; cancel it. Power off on any exit, so a failed
    # setup doesn't leave the driver running (driver-run.sh also does, which is harmless).
    # A run SSM killed at its timeout never uploaded its results: upload what it left, marked killed, before clearing.
    return f"""export HOME=/root PATH=/usr/local/bin:/usr/bin:/bin:/usr/local/go/bin
export LAB_DEADLINE=$(( $(date +%s) + {EXECUTION_TIMEOUT_S - DEADLINE_MARGIN_S} ))
set -euo pipefail
shutdown -c 2>/dev/null || true
trap 'shutdown -h +1 >/dev/null 2>&1' EXIT
rm -f /run/cnm-perf-lab-cancel /run/cnm-perf-lab-finishing
for d in /var/lib/cnm-perf-lab/runs/*/; do
  [ -d "$d" ] && [ ! -e "$d/EXIT_CODE" ] || continue
  echo killed > "$d/EXIT_CODE"
  aws s3 cp --only-show-errors --recursive "$d" "s3://{BUCKET}/runs/$(basename "$d")/" || true
done
mkdir -p {lab} && cd {lab}
aws s3 cp --only-show-errors s3://{BUCKET}/{LAB_TARBALL_KEY} cnm-perf-lab.tgz
rm -rf cnm-perf-lab /var/lib/cnm-perf-lab/runs /opt/cnm-perf-lab && tar xzf cnm-perf-lab.tgz
ln -sfn {lab}/cnm-perf-lab /opt/cnm-perf-lab
/opt/cnm-perf-lab/driver-setup.sh < /dev/null
{refs}cd /opt/cnm-perf-lab
LAB_PROVISION_REPO=/opt/datadog-agent LAB_WORK=/var/lib/cnm-perf-lab LAB_RESULTS_S3=s3://{BUCKET}/runs \\
  LAB_RUN_ID={shlex.quote(run_id)} LAB_SHUTDOWN_AFTER=1 LAB_NDQ_CHECK={1 if check_ndq else 0} ./driver-run.sh {shlex.quote(mode)}
"""


def _print_driver_log_tail(aws: _Aws, run_id: str, seen: int) -> int:
    """Print the new lines of the driver log the driver uploads every few minutes; returns the lines seen so far."""
    try:
        body = aws.client("s3").get_object(Bucket=BUCKET, Key=f"runs/{run_id}/driver.log")["Body"].read()
    except Exception:
        return seen
    lines = body.decode(errors="replace").splitlines()
    for line in lines[seen:]:
        print(f"  | {line}")
    return len(lines)


def _download_prefix(aws: _Aws, prefix: str, dest: Path) -> None:
    s3 = aws.client("s3")
    for page in s3.get_paginator("list_objects_v2").paginate(Bucket=BUCKET, Prefix=prefix):
        for obj in page.get("Contents", []):
            target = dest / obj["Key"][len(prefix) :]
            target.parent.mkdir(parents=True, exist_ok=True)
            s3.download_file(BUCKET, obj["Key"], str(target))


def _print_ssm_output(aws: _Aws, command_id: str, instance_id: str) -> None:
    """The tail of the SSM command's output, for failures before driver-run.sh started its log (setup, download)."""
    try:
        inv = aws.client("ssm").get_command_invocation(CommandId=command_id, InstanceId=instance_id)
    except Exception as e:
        print(f"could not read the SSM command's output: {e}")
        return
    for stream in ("StandardOutputContent", "StandardErrorContent"):
        lines = inv.get(stream, "").splitlines()[-40:]
        if lines:
            print(f"SSM {stream} (last {len(lines)} lines):")
            for line in lines:
                print(f"  | {line}")


def _runs_in_progress(aws: _Aws, instance_id: str) -> list[str]:
    commands = aws.client("ssm").list_commands(InstanceId=instance_id, MaxResults=50)["Commands"]
    return [
        c["CommandId"]
        for c in commands
        if c.get("Comment", "").startswith(RUN_COMMENT) and c["Status"] in ("Pending", "InProgress", "Delayed")
    ]


def _send_lab_metrics(agg_path: Path, run_id: str, variant_tags: dict[str, list[str]] | None = None) -> int:
    """Send the mean of every aggregated metric, per tracer mode and variant, to the Datadog org of the CI."""
    from tasks.libs.common.datadog_api import create_gauge, send_metrics

    agg = json.loads(agg_path.read_text())
    timestamp = int(time.time())
    base_tags = [
        f"run_id:{run_id}",
        f"pipeline_id:{os.environ.get('CI_PIPELINE_ID', 'local')}",
        f"git_sha:{os.environ.get('CI_COMMIT_SHA', 'unknown')[:12]}",
    ]
    series = []
    for mode, by_mode in agg.items():
        for variant, values in by_mode.get("variants", {}).items():
            tags = base_tags + [f"mode:{mode}", f"variant:{variant}"] + (variant_tags or {}).get(variant, [])
            for name, value in values.items():
                mean = value.get("mean") if isinstance(value, dict) else None
                if isinstance(mean, int | float) and not isinstance(mean, bool) and math.isfinite(mean):
                    series.append(create_gauge(f"{METRIC_PREFIX}.{name}", timestamp, mean, tags))
    for offset in range(0, len(series), 500):
        send_metrics(series[offset : offset + 500])
    return len(series)


def _variant_tags(role: str, ref: str) -> list[str]:
    """Tags for a pinned image; the full repo@sha256:... doesn't survive as a tag value."""
    repo, _, digest = ref.partition("@")
    tags = [f"variant_role:{role}", f"variant_image:{repo}"]
    if digest:
        tags.append(f"variant_digest:{digest.removeprefix('sha256:')[:12]}")
    return tags


@task(
    help={
        "mode": "smoke (one short window) or rounds (the full Latin-square run)",
        "variants": "release (latest GA, the anchor release and main's nightly) or lab (the lab's own builds)",
        "refs": "Optional REFS.txt (\"<variant> <image ref>\" lines); overrides --variants",
        "results_dir": "Where to download the run's results (job artifacts)",
        "send_metrics": "Send the aggregated means to Datadog (needs DD_API_KEY)",
    }
)
def run(
    ctx: Context, mode="smoke", variants="release", refs=None, results_dir="cnm-perf-lab-results", send_metrics=True
):
    """Run the CNM agent perf lab on its driver instance and collect the results."""
    if mode not in ("smoke", "rounds"):
        raise Exit("mode must be smoke or rounds", code=2)
    if variants not in ("release", "lab"):
        raise Exit("variants must be release or lab", code=2)
    aws = _Aws()
    job = os.environ.get("CI_JOB_ID")
    run_id = f"ci-{job}-{mode}" if job else f"local-{int(time.time())}-{mode}"
    aws.client("s3").head_object(Bucket=BUCKET, Key=LAB_TARBALL_KEY)  # fails early if the lab isn't published

    # The lab's own builds come from its refs/default.txt; anything else is sent as a REFS.txt
    refs_key, variant_tags, check_ndq = None, {}, variants == "lab" and not refs
    if refs or variants == "release":
        if not refs:
            resolved = _release_refs()
            refs = "cnm-perf-lab-refs.txt"
            Path(refs).write_text("".join(f"{v} {ref}\n" for v, (_, ref) in resolved.items()))
            variant_tags = {v: _variant_tags(role, ref) for v, (role, ref) in resolved.items()}
        refs_key = f"runs/{run_id}/REFS.txt"
        aws.client("s3").upload_file(refs, BUCKET, refs_key)

    instance_id = _driver_id(aws)
    busy = _runs_in_progress(aws, instance_id)
    if busy:
        raise Exit(f"another lab run is in progress on the driver (SSM {', '.join(busy)}); not starting", code=1)
    MARKER.write_text(f"{instance_id}\n")  # cancel stops the driver if no run was handed over yet
    staging_key = os.environ.get("CNM_PERF_LAB_STAGING_API_KEY")
    if staging_key:
        aws.client("secretsmanager").put_secret_value(
            SecretId=LAB_KEYS_SECRET, SecretString=json.dumps({"api_key": staging_key})
        )
        KEY_WRITTEN.touch()
    else:
        print(f"CNM_PERF_LAB_STAGING_API_KEY not set; the driver uses the key already in {LAB_KEYS_SECRET}")
    try:
        _run_on_driver(aws, instance_id, run_id, mode, refs_key, check_ndq, results_dir, send_metrics, variant_tags)
    finally:
        _clear_staging_key(aws)


def _run_on_driver(aws, instance_id, run_id, mode, refs_key, check_ndq, results_dir, send_metrics, variant_tags):
    _start_driver(aws, instance_id)
    command_id = aws.client("ssm").send_command(
        InstanceIds=[instance_id],
        DocumentName="AWS-RunShellScript",
        Comment=f"{RUN_COMMENT} {run_id}",
        TimeoutSeconds=600,
        OutputS3BucketName=BUCKET,
        OutputS3KeyPrefix=f"ssm/{run_id}",
        Parameters={
            "commands": [_driver_command(run_id, mode, refs_key, check_ndq)],
            "executionTimeout": [str(EXECUTION_TIMEOUT_S)],
        },
    )["Command"]["CommandId"]
    MARKER.write_text(f"{instance_id} {command_id}\n")
    print(f"run {run_id}: SSM command {command_id} on {instance_id}")

    status, seen, last_tail = "Pending", 0, 0.0
    while status in ("Pending", "InProgress", "Delayed", "Cancelling"):
        time.sleep(60)
        try:
            status = aws.client("ssm").get_command_invocation(CommandId=command_id, InstanceId=instance_id)["Status"]
        except aws.client("ssm").exceptions.InvocationDoesNotExist:
            continue
        if time.time() - last_tail > 300:
            seen, last_tail = _print_driver_log_tail(aws, run_id, seen), time.time()
    _print_driver_log_tail(aws, run_id, seen)
    print(f"run {run_id}: SSM command finished with status {status}")

    dest = Path(results_dir)
    _download_prefix(aws, f"runs/{run_id}/", dest)
    exit_code = (dest / "EXIT_CODE").read_text().strip() if (dest / "EXIT_CODE").exists() else "unknown"
    if status == "TimedOut":
        # SSM killed the run before its teardown, so the driver won't power itself off. The next run destroys the
        # cluster this one left; stop the driver until then.
        aws.client("ec2").stop_instances(InstanceIds=[instance_id])
        print(f"run {run_id}: timed out before teardown; stopped the driver, and the next run destroys its cluster")
    if status != "Success" or exit_code != "0":
        _print_ssm_output(aws, command_id, instance_id)
        _download_prefix(aws, f"ssm/{run_id}/", dest / "ssm")
        raise Exit(f"the lab run failed (SSM {status}, driver exit {exit_code}); see {dest}/", code=1)

    if send_metrics and (dest / "agg.json").exists():
        print(f"sent {_send_lab_metrics(dest / 'agg.json', run_id, variant_tags)} metrics")


@task
def cancel(ctx: Context):
    """Stop a lab run started by `run` (for after_script): the driver then destroys its cluster and powers off."""
    if not MARKER.exists():
        print("no lab run was started")
        return
    instance_id, *rest = MARKER.read_text().split()
    aws = _Aws()
    if not rest:
        # Started the driver but never handed it a run: stop it, unless a run from elsewhere is using it
        if _runs_in_progress(aws, instance_id):
            print(f"no run was handed to {instance_id}, and another run is using it; leaving it running")
            return
        aws.client("ec2").stop_instances(InstanceIds=[instance_id])
        print(f"no run was handed over; stopped the driver {instance_id}")
        _clear_staging_key(aws)
        return
    command_id = rest[0]
    # The run clears the cancel flag when it starts, so wait until it has started before setting it
    for _ in range(12):
        status = aws.client("ssm").get_command_invocation(CommandId=command_id, InstanceId=instance_id)["Status"]
        if status not in ("Pending", "Delayed"):
            break
        time.sleep(10)
    if status not in ("Pending", "InProgress", "Delayed"):
        print(f"the lab run already ended ({status})")
        _clear_staging_key(aws)
        return
    # Interrupt the current lab step rather than cancelling the SSM command, so driver-run.sh's exit trap still destroys
    # the cluster, uploads the results and powers the instance off. The flag also stops a run still in setup from
    # starting its first step. Once teardown has begun (the finishing flag), leave the destroy step alone.
    aws.client("ssm").send_command(
        InstanceIds=[instance_id],
        DocumentName="AWS-RunShellScript",
        Comment="cnm-perf-lab cancel",
        Parameters={
            "commands": [
                "touch /run/cnm-perf-lab-cancel; "
                "[ -e /run/cnm-perf-lab-finishing ] || pkill -TERM -f '^timeout --kill-after' || true"
            ]
        },
    )
    print(f"asked the driver {instance_id} to stop the run; it destroys the cluster and powers off on its own")
    # The driver read the key when its run started, so the run doesn't need the secret any more
    _clear_staging_key(aws)
