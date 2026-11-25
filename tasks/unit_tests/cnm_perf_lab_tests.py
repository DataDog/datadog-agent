"""
Tests for tasks/cnm_perf_lab.py. AWS calls go to real boto3 clients wrapped in botocore's Stubber, which rejects any
call whose operation or parameters don't match the service's API model.
"""

import io
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import boto3
from botocore.response import StreamingBody
from botocore.stub import ANY, Stubber
from invoke import Exit, MockContext

from tasks import cnm_perf_lab as lab

INSTANCE = "i-0123456789abcdef0"
COMMAND = "11111111-2222-3333-4444-555555555555"


def _cid(c):
    return f"{c * 8}-0000-0000-0000-000000000000"


class _FakeAws:
    """Stands in for lab._Aws: one stubbed client per service, kept across calls like the real one."""

    def __init__(self):
        self.clients, self.stubs = {}, {}
        self.uploads, self.downloads = [], {}

    def client(self, name):
        if name not in self.clients:
            c = boto3.client(name, region_name=lab.REGION, aws_access_key_id="testing", aws_secret_access_key="testing")
            self.stubs[name] = Stubber(c)
            self.stubs[name].activate()
            if name == "s3":
                # Transfers are s3transfer's business; record them instead
                c.upload_file = lambda path, bucket, key: self.uploads.append((Path(path).read_text(), bucket, key))
                c.download_file = lambda bucket, key, path: Path(path).write_text(self.downloads[key])
            self.clients[name] = c
        return self.clients[name]

    def stub(self, name):
        self.client(name)
        return self.stubs[name]

    def assert_done(self):
        for stub in self.stubs.values():
            stub.assert_no_pending_responses()


def _instances(state):
    return {"Reservations": [{"Instances": [{"InstanceId": INSTANCE, "State": {"Name": state, "Code": 16}}]}]}


def _body(text):
    data = text.encode()
    return StreamingBody(io.BytesIO(data), len(data))


class _InTempDir(unittest.TestCase):
    def setUp(self):
        self._cwd = os.getcwd()
        self._tmp = tempfile.TemporaryDirectory()
        os.chdir(self._tmp.name)
        sleep = patch("time.sleep")
        sleep.start()
        self.addCleanup(sleep.stop)

    def tearDown(self):
        os.chdir(self._cwd)
        self._tmp.cleanup()


class TestAwsSession(unittest.TestCase):
    def test_assumes_the_ci_role_with_its_external_id_and_refreshes(self):
        sts = boto3.client("sts", region_name=lab.REGION, aws_access_key_id="x", aws_secret_access_key="x")
        creds = {
            "Credentials": {
                "AccessKeyId": "AKIAEXAMPLEEXAMPLE12",
                "SecretAccessKey": "secret",
                "SessionToken": "token",
                "Expiration": "2030-01-01T00:00:00Z",
            }
        }
        expected = {"RoleArn": lab.CI_ROLE, "ExternalId": lab.CI_ROLE_EXTERNAL_ID, "RoleSessionName": "cnm-perf-lab-42"}
        with Stubber(sts) as stub, patch("boto3.client", return_value=sts), patch.dict(os.environ, {"CI_JOB_ID": "42"}):
            stub.add_response("assume_role", creds, expected)
            stub.add_response("assume_role", creds, expected)
            aws = lab._Aws()
            with patch("time.time", return_value=1000.0):
                aws.client("ec2")
                aws.client("ssm")  # same session
            with patch("time.time", return_value=1000.0 + lab.SESSION_REFRESH_S + 1):
                aws.client("ec2")  # re-assumed
            stub.assert_no_pending_responses()


class TestDriver(_InTempDir):
    def test_driver_id_needs_exactly_one_instance(self):
        aws = _FakeAws()
        filters = {
            "Filters": [
                {"Name": "tag:service", "Values": [lab.DRIVER_TAG]},
                {"Name": "instance-state-name", "Values": ["pending", "running", "stopping", "stopped"]},
            ]
        }
        aws.stub("ec2").add_response("describe_instances", _instances("stopped"), filters)
        self.assertEqual(lab._driver_id(aws), INSTANCE)
        two = _instances("stopped")
        two["Reservations"].append(_instances("running")["Reservations"][0])
        aws.stub("ec2").add_response("describe_instances", two, filters)
        with self.assertRaises(Exit):
            lab._driver_id(aws)
        aws.assert_done()

    def test_start_driver_starts_a_stopped_instance_and_waits_for_ssm(self):
        aws = _FakeAws()
        ec2, ssm = aws.stub("ec2"), aws.stub("ssm")
        ec2.add_response("describe_instances", _instances("stopped"), {"InstanceIds": [INSTANCE]})
        ec2.add_response("start_instances", {}, {"InstanceIds": [INSTANCE]})
        ec2.add_response("describe_instances", _instances("running"), {"InstanceIds": [INSTANCE]})  # waiter
        info = {"Filters": [{"Key": "InstanceIds", "Values": [INSTANCE]}]}
        ssm.add_response("describe_instance_information", {"InstanceInformationList": []}, info)
        ssm.add_response(
            "describe_instance_information",
            {"InstanceInformationList": [{"InstanceId": INSTANCE, "PingStatus": "Online"}]},
            info,
        )
        lab._start_driver(aws, INSTANCE)
        aws.assert_done()

    def test_runs_in_progress_only_counts_lab_runs(self):
        aws = _FakeAws()
        commands = [
            {"CommandId": _cid("a"), "Comment": f"{lab.RUN_COMMENT} ci-1-smoke", "Status": "InProgress"},
            {"CommandId": _cid("b"), "Comment": f"{lab.RUN_COMMENT} ci-2-smoke", "Status": "Success"},
            {"CommandId": _cid("c"), "Comment": "cnm-perf-lab cancel", "Status": "InProgress"},
            {"CommandId": _cid("d"), "Status": "Pending"},
        ]
        aws.stub("ssm").add_response(
            "list_commands", {"Commands": commands}, {"InstanceId": INSTANCE, "MaxResults": 50}
        )
        self.assertEqual(lab._runs_in_progress(aws, INSTANCE), [_cid("a")])
        aws.assert_done()

    def test_driver_command_is_valid_shell(self):
        for refs_key in (None, "runs/ci-1-smoke/REFS.txt"):
            script = lab._driver_command("ci-1-smoke", "smoke", refs_key, check_ndq=False)
            subprocess.run(["bash", "-n"], input=script, text=True, check=True)
            self.assertIn("shutdown -c", script)
            self.assertIn("rm -f /run/cnm-perf-lab-cancel /run/cnm-perf-lab-finishing", script)


class TestRun(_InTempDir):
    RUN_ID = "ci-42-smoke"
    REFS = {"A": ("ga", "datadog/agent@sha256:" + "a" * 64)}

    def _expect_start(self, aws, staging_key=None):
        ec2, ssm = aws.stub("ec2"), aws.stub("ssm")
        aws.stub("s3").add_response("head_object", {}, {"Bucket": lab.BUCKET, "Key": lab.LAB_TARBALL_KEY})
        ec2.add_response("describe_instances", _instances("stopped"), {"Filters": ANY})
        ssm.add_response("list_commands", {"Commands": []}, {"InstanceId": INSTANCE, "MaxResults": 50})
        if staging_key:
            aws.stub("secretsmanager").add_response(
                "put_secret_value",
                {},
                {"SecretId": lab.LAB_KEYS_SECRET, "SecretString": json.dumps({"api_key": staging_key})},
            )
        ec2.add_response("describe_instances", _instances("running"), {"InstanceIds": [INSTANCE]})
        ec2.add_response("describe_instances", _instances("running"), {"InstanceIds": [INSTANCE]})
        ssm.add_response(
            "describe_instance_information",
            {"InstanceInformationList": [{"InstanceId": INSTANCE, "PingStatus": "Online"}]},
            {"Filters": ANY},
        )
        ssm.add_response(
            "send_command",
            {"Command": {"CommandId": COMMAND}},
            {
                "InstanceIds": [INSTANCE],
                "DocumentName": "AWS-RunShellScript",
                "Comment": f"{lab.RUN_COMMENT} {self.RUN_ID}",
                "TimeoutSeconds": 600,
                "OutputS3BucketName": lab.BUCKET,
                "OutputS3KeyPrefix": f"ssm/{self.RUN_ID}",
                "Parameters": {"commands": ANY, "executionTimeout": [str(lab.EXECUTION_TIMEOUT_S)]},
            },
        )

    def _expect_results(self, aws, prefix, files):
        aws.stub("s3").add_response(
            "list_objects_v2",
            {"Contents": [{"Key": prefix + name} for name in files]},
            {"Bucket": lab.BUCKET, "Prefix": prefix},
        )
        aws.downloads.update({prefix + name: text for name, text in files.items()})

    def _run(self, aws, env=None, **kw):
        with (
            patch.object(lab, "_Aws", return_value=aws),
            patch.object(lab, "_release_refs", return_value=self.REFS),
            patch.dict(os.environ, {"CI_JOB_ID": "42", **(env or {})}),
        ):
            if not env:
                os.environ.pop("CNM_PERF_LAB_STAGING_API_KEY", None)
            lab.run(MockContext(), mode="smoke", variants="release", **kw)

    def test_hands_the_staging_key_to_the_driver_before_starting_it(self):
        aws = _FakeAws()
        self._expect_start(aws, staging_key="k" * 32)
        invocation = {"CommandId": COMMAND, "InstanceId": INSTANCE}
        aws.stub("ssm").add_response("get_command_invocation", {"Status": "Success"}, invocation)
        log_key = {"Bucket": lab.BUCKET, "Key": f"runs/{self.RUN_ID}/driver.log"}
        aws.stub("s3").add_client_error("get_object", "NoSuchKey", expected_params=log_key)
        aws.stub("s3").add_client_error("get_object", "NoSuchKey", expected_params=log_key)
        self._expect_results(aws, f"runs/{self.RUN_ID}/", {"EXIT_CODE": "0\n"})
        self._run(aws, env={"CNM_PERF_LAB_STAGING_API_KEY": "k" * 32}, send_metrics=False)
        aws.assert_done()

    def test_successful_run_collects_results_and_sends_metrics(self):
        aws = _FakeAws()
        self._expect_start(aws)
        invocation = {"CommandId": COMMAND, "InstanceId": INSTANCE}
        aws.stub("ssm").add_response("get_command_invocation", {"Status": "Success"}, invocation)
        log_key = {"Bucket": lab.BUCKET, "Key": f"runs/{self.RUN_ID}/driver.log"}
        aws.stub("s3").add_response("get_object", {"Body": _body("a\nb\n")}, log_key)
        aws.stub("s3").add_response("get_object", {"Body": _body("a\nb\nc\n")}, log_key)
        agg = {"kprobe": {"variants": {"A": {"sp_cores": {"mean": 0.19}, "bad": {"mean": float("nan")}}}}}
        self._expect_results(aws, f"runs/{self.RUN_ID}/", {"EXIT_CODE": "0\n", "agg.json": json.dumps(agg)})
        with patch("tasks.libs.common.datadog_api.send_metrics") as send:
            self._run(aws)
        aws.assert_done()
        self.assertEqual(aws.uploads, [(f"A {self.REFS['A'][1]}\n", lab.BUCKET, f"runs/{self.RUN_ID}/REFS.txt")])
        self.assertEqual(lab.MARKER.read_text().split(), [INSTANCE, COMMAND])
        (series,) = send.call_args.args
        self.assertEqual(len(series), 1)  # the NaN mean is dropped
        self.assertIn("variant_digest:" + "a" * 12, series[0]["tags"])

    def test_failed_setup_prints_and_downloads_the_ssm_output(self):
        aws = _FakeAws()
        self._expect_start(aws)
        invocation = {"CommandId": COMMAND, "InstanceId": INSTANCE}
        ssm, s3 = aws.stub("ssm"), aws.stub("s3")
        ssm.add_response("get_command_invocation", {"Status": "Failed"}, invocation)
        log_key = {"Bucket": lab.BUCKET, "Key": f"runs/{self.RUN_ID}/driver.log"}
        s3.add_client_error("get_object", "NoSuchKey", expected_params=log_key)
        s3.add_client_error("get_object", "NoSuchKey", expected_params=log_key)
        self._expect_results(aws, f"runs/{self.RUN_ID}/", {})
        ssm.add_response(
            "get_command_invocation",
            {"Status": "Failed", "StandardOutputContent": "setup", "StandardErrorContent": "boom"},
            invocation,
        )
        self._expect_results(aws, f"ssm/{self.RUN_ID}/", {"stderr": "boom"})
        with self.assertRaises(Exit):
            self._run(aws, send_metrics=False)
        aws.assert_done()
        self.assertEqual(Path("cnm-perf-lab-results/ssm/stderr").read_text(), "boom")

    def test_refuses_to_start_while_another_run_is_in_progress(self):
        aws = _FakeAws()
        aws.stub("s3").add_response("head_object", {}, {"Bucket": lab.BUCKET, "Key": lab.LAB_TARBALL_KEY})
        aws.stub("ec2").add_response("describe_instances", _instances("running"), {"Filters": ANY})
        aws.stub("ssm").add_response(
            "list_commands",
            {
                "Commands": [
                    {"CommandId": _cid("x"), "Comment": f"{lab.RUN_COMMENT} ci-1-smoke", "Status": "InProgress"}
                ]
            },
            {"InstanceId": INSTANCE, "MaxResults": 50},
        )
        with self.assertRaises(Exit):
            self._run(aws, send_metrics=False)
        aws.assert_done()
        self.assertFalse(lab.MARKER.exists())


class TestCancel(_InTempDir):
    def _cancel(self, aws):
        with patch.object(lab, "_Aws", return_value=aws):
            lab.cancel(MockContext())

    def test_no_run_started(self):
        self._cancel(_FakeAws())  # makes no AWS call

    def test_stops_a_driver_that_never_got_the_run(self):
        lab.MARKER.write_text(f"{INSTANCE}\n")
        aws = _FakeAws()
        aws.stub("ssm").add_response("list_commands", {"Commands": []}, {"InstanceId": INSTANCE, "MaxResults": 50})
        aws.stub("ec2").add_response("stop_instances", {}, {"InstanceIds": [INSTANCE]})
        self._cancel(aws)
        aws.assert_done()

    def test_waits_for_a_pending_run_then_interrupts_it(self):
        lab.MARKER.write_text(f"{INSTANCE} {COMMAND}\n")
        aws = _FakeAws()
        ssm = aws.stub("ssm")
        invocation = {"CommandId": COMMAND, "InstanceId": INSTANCE}
        ssm.add_response("get_command_invocation", {"Status": "Pending"}, invocation)
        ssm.add_response("get_command_invocation", {"Status": "InProgress"}, invocation)
        ssm.add_response(
            "send_command",
            {"Command": {"CommandId": _cid("f")}},
            {
                "InstanceIds": [INSTANCE],
                "DocumentName": "AWS-RunShellScript",
                "Comment": "cnm-perf-lab cancel",
                "Parameters": {"commands": ANY},
            },
        )
        self._cancel(aws)
        aws.assert_done()

    def test_leaves_an_ended_run_alone(self):
        lab.MARKER.write_text(f"{INSTANCE} {COMMAND}\n")
        aws = _FakeAws()
        aws.stub("ssm").add_response(
            "get_command_invocation", {"Status": "Success"}, {"CommandId": COMMAND, "InstanceId": INSTANCE}
        )
        self._cancel(aws)
        aws.assert_done()


class TestRefs(unittest.TestCase):
    def test_pin(self):
        calls = []

        class Resp:
            headers = {"Docker-Content-Digest": "sha256:" + "d" * 64}

            def read(self):
                return b'{"token": "t"}'

        def hub(url, *args, **kwargs):
            calls.append(url)
            return Resp()

        with patch.object(lab, "_hub_get", side_effect=hub):
            for ref, repo, tag in [
                ("datadog/agent:7.82.1", "datadog/agent", "7.82.1"),
                ("datadog/agent", "datadog/agent", "latest"),
                ("docker.io/datadog/agent:7", "datadog/agent", "7"),
                ("busybox", "library/busybox", "latest"),
            ]:
                self.assertEqual(lab._pin(ref), f"{repo}@sha256:" + "d" * 64)
                self.assertTrue(calls[-1].endswith(f"/v2/{repo}/manifests/{tag}"), calls[-1])
            calls.clear()
            for ref in ("registry.ddbuild.io/datadog-agent:dev", "datadog/agent@sha256:" + "e" * 64):
                self.assertEqual(lab._pin(ref), ref)
            self.assertEqual(calls, [])

    def test_variant_tags(self):
        self.assertEqual(
            lab._variant_tags("ga", "datadog/agent@sha256:" + "c" * 64),
            ["variant_role:ga", "variant_image:datadog/agent", "variant_digest:" + "c" * 12],
        )


if __name__ == "__main__":
    unittest.main()
