import contextlib
import json
import plistlib
import shlex
import tempfile
import unittest
import urllib.error
from pathlib import Path
from unittest.mock import MagicMock, patch

from invoke import Exit

from tasks import eudm_simulator as eudm

COMMIT = "a" * 40


class TestCheckoutCommit(unittest.TestCase):
    def test_loose_packed_and_detached_revisions(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            git_dir = root / ".git"
            (git_dir / "refs/heads").mkdir(parents=True)
            (git_dir / "HEAD").write_text("ref: refs/heads/eudm\n")
            loose = git_dir / "refs/heads/eudm"
            loose.write_text(COMMIT + "\n")
            self.assertEqual(eudm._checkout_commit(root), COMMIT)
            loose.unlink()
            (git_dir / "packed-refs").write_text(f"# packed refs\n{COMMIT} refs/heads/eudm\n")
            self.assertEqual(eudm._checkout_commit(root), COMMIT)
            (git_dir / "HEAD").write_text(COMMIT + "\n")
            self.assertEqual(eudm._checkout_commit(root), COMMIT)

    def test_worktree_uses_common_refs(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            common = root / "main/.git"
            worktree = root / "checkout"
            metadata = common / "worktrees/eudm"
            metadata.mkdir(parents=True)
            worktree.mkdir()
            (worktree / ".git").write_text("gitdir: ../main/.git/worktrees/eudm\n")
            (metadata / "commondir").write_text("../..\n")
            (metadata / "HEAD").write_text("ref: refs/heads/eudm\n")
            (common / "packed-refs").write_text(f"{COMMIT} refs/heads/eudm\n")
            self.assertEqual(eudm._checkout_commit(worktree), COMMIT)

    def test_missing_checkout_requires_explicit_identity(self):
        with tempfile.TemporaryDirectory() as directory, self.assertRaisesRegex(Exit, "supply --commit"):
            eudm._checkout_commit(Path(directory))


class TestInstall(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="eudm install tests ")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.ctx = MagicMock()
        self.events = []
        self.stack = contextlib.ExitStack()
        self.addCleanup(self.stack.close)
        self.stack.enter_context(patch.object(eudm, "__file__", str(self.root / "tasks/eudm_simulator.py")))
        self.stack.enter_context(patch.object(eudm, "_macos_preflight"))
        self.stack.enter_context(patch.object(eudm, "_capture_api_reader"))
        self.build = self.stack.enter_context(patch.object(eudm, "_build_capture_binaries"))
        self.stage = self.stack.enter_context(patch.object(eudm, "_stage_capture_binaries", side_effect=self.staged))
        self.ready = self.stack.enter_context(patch.object(eudm, "_wait_for_capture_apis"))
        self.print = self.stack.enter_context(patch("builtins.print"))
        self.ctx.run.side_effect = self.install_script

    def staged(self, _ctx, _binaries, staging, _installed):
        self.events.append("staged")
        for name in eudm._CAPTURE_TARGETS:
            binary = staging / name
            binary.write_bytes(b"checked binary")
            binary.chmod(0o755)

    def install_script(self, command, **kwargs):
        self.events.append("installed")
        argv = shlex.split(command)
        self.assertEqual(argv[:2], ["sudo", "/bin/sh"])
        self.assertTrue(kwargs["pty"])
        self.script = Path(argv[2]).read_text()

    def test_prepare_only_never_requests_privileges_or_changes_services(self):
        eudm.install.body(self.ctx, prepare_only=True, commit=COMMIT)
        self.assertEqual(self.events, ["staged"])
        self.ctx.run.assert_not_called()
        self.ready.assert_not_called()
        self.assertEqual((self.root / "bin/eudm-simulator/eudm-simulator").read_bytes(), b"checked binary")

    def test_build_failure_cannot_install(self):
        self.build.side_effect = Exit("build failed")
        with self.assertRaisesRegex(Exit, "build failed"):
            eudm.install.body(self.ctx, commit=COMMIT)
        self.ctx.run.assert_not_called()
        self.stage.assert_not_called()
        self.ready.assert_not_called()

    def test_runtime_failure_cannot_install(self):
        self.stage.side_effect = Exit("runtime incompatible")
        with self.assertRaisesRegex(Exit, "runtime incompatible"):
            eudm.install.body(self.ctx, commit=COMMIT)
        self.ctx.run.assert_not_called()
        self.ready.assert_not_called()

    def test_stage_both_before_replacement_and_restart_only_core(self):
        eudm.install.body(self.ctx, commit=COMMIT)
        self.assertEqual(self.events, ["staged", "installed"])
        lines = self.script.splitlines()
        commands = [shlex.split(line) for line in lines[2:]]
        self.assertEqual(
            [command[0] for command in commands], ["/usr/bin/install"] * 2 + ["/bin/mv"] * 2 + ["/bin/launchctl"]
        )
        self.assertEqual(commands[-1], ["/bin/launchctl", "kickstart", "-k", "system/com.datadoghq.agent"])
        self.assertNotIn("datadog.yaml", self.script)
        self.assertNotIn("auth_token", self.script)
        self.assertNotIn("sysprobe", self.script)
        self.assertNotIn("trace-agent", self.script)
        self.ready.assert_called_once()
        self.print.assert_any_call(
            "Capture with: ./bin/eudm-simulator/eudm-simulator capture --cfgpath /opt/datadog-agent/etc/datadog.yaml --timeout 70m --output /private/tmp/eudm-macos-baseline"
        )

    def test_failed_privilege_request_does_not_claim_readiness(self):
        self.ctx.run.side_effect = Exit("sudo failed")
        with self.assertRaisesRegex(Exit, "sudo failed"):
            eudm.install.body(self.ctx, commit=COMMIT)
        self.ready.assert_not_called()

    def test_bad_commit_rejected_before_build(self):
        with self.assertRaisesRegex(Exit, "40 lowercase"):
            eudm.install.body(self.ctx, commit="not-a-revision")
        self.build.assert_not_called()


class TestPreflight(unittest.TestCase):
    def test_unsupported_platform_and_root_rejected_before_build(self):
        with patch.object(eudm.sys, "platform", "win32"), self.assertRaisesRegex(Exit, "macOS only"):
            eudm._macos_preflight(Path("/unused"), Path("/unused"))
        with patch.object(eudm.sys, "platform", "darwin"), patch.object(eudm.os, "geteuid", return_value=0):
            with self.assertRaisesRegex(Exit, "normal user"):
                eudm._macos_preflight(Path("/unused"), Path("/unused"))

    def test_nonstandard_service_is_not_replaced(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for relative in (
                "bin/agent/agent",
                "embedded/bin/process-agent",
                "embedded/lib/libdatadog-agent-rtloader.dylib",
            ):
                path = root / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.touch()
            plist = root / "service.plist"
            plist.write_bytes(plistlib.dumps({"Label": "unrelated.service", "ProgramArguments": ["/other/service"]}))
            with patch.object(eudm.sys, "platform", "darwin"), patch.object(eudm.os, "geteuid", return_value=501):
                with (
                    patch.object(eudm.shutil, "which", return_value="/tool"),
                    self.assertRaisesRegex(Exit, "standard macOS"),
                ):
                    eudm._macos_preflight(root, plist)

    def test_remote_api_address_rejected_without_sending_token(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "etc").mkdir()
            (root / "etc/datadog.yaml").write_text("cmd_host: 192.0.2.1\napi_key: private-test-key\n")
            with patch.object(eudm.urllib.request, "build_opener") as opener:
                with self.assertRaisesRegex(Exit, "local API settings"):
                    eudm._capture_api_reader(root)
                opener.assert_not_called()

    def test_redirects_cannot_forward_local_authentication(self):
        for code in (301, 302, 303, 307, 308):
            with self.subTest(code=code):
                handler = eudm._NoCaptureRedirects()
                opener = eudm.urllib.request.build_opener(eudm.urllib.request.ProxyHandler({}), handler)
                request = eudm.urllib.request.Request(
                    "https://localhost:5001/agent/eudm-capture/capabilities",
                    headers={"Authorization": "Bearer private-test-token"},
                )
                with (
                    patch.object(handler, "redirect_request", wraps=handler.redirect_request) as redirect,
                    patch.object(opener, "open") as send,
                ):
                    with self.assertRaises(urllib.error.HTTPError) as error:
                        opener.error(
                            "https",
                            request,
                            MagicMock(),
                            code,
                            "Redirect",
                            {"location": "https://example.invalid/capture"},
                        )
                    self.assertEqual(error.exception.code, code)
                    redirect.assert_called_once()
                    send.assert_not_called()

    def test_api_reader_matches_agent_address_precedence_and_port_fallback(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "etc").mkdir()
            (root / "etc/datadog.yaml").write_text(
                "cmd_host: 127.0.0.2\nipc_address: 127.0.0.1\nprocess_config:\n  cmd_port: 0\n"
            )
            (root / "etc/auth_token").write_text("private-test-token")
            with (
                patch.object(eudm.ssl, "create_default_context"),
                patch.object(eudm.urllib.request, "build_opener") as build,
            ):
                response = build.return_value.open.return_value.__enter__.return_value
                response.read.return_value = b"{}"
                read = eudm._capture_api_reader(root)
                read("process-agent")
                request = build.return_value.open.call_args.args[0]
                self.assertEqual(request.full_url, "https://127.0.0.1:6162/eudm-capture/capabilities")
                response.read.assert_called_once_with(65537)


class TestReadiness(unittest.TestCase):
    @staticmethod
    def status(role, commit=COMMIT):
        streams = (
            ["metrics", "metadata", "agent_inventory", "host_inventory", "software"]
            if role == "core-agent"
            else ["processes"]
        )
        capabilities = [{"stream": stream, "cadence": 15_000_000_000} for stream in streams]
        if role == "core-agent":
            capabilities[0]["metric_schedules"] = [
                {"family": "cpu", "cadence": 15_000_000_000},
                {"family": "memory", "cadence": 15_000_000_000},
                {"family": "battery", "cadence": 300_000_000_000},
            ]
        return {
            "protocol_version": 2,
            "producer": {"role": role, "commit": commit, "instance_id": "opaque"},
            "capabilities": capabilities,
        }

    def test_waits_for_new_build_and_running_streams(self):
        calls = 0

        def read(role):
            nonlocal calls
            calls += 1
            return self.status(role, commit="b" * 40 if calls <= 2 else COMMIT)

        with patch.object(eudm.time, "sleep"), patch("builtins.print"):
            eudm._wait_for_capture_apis(read, COMMIT)
        self.assertEqual(calls, 4)

    def test_optional_hardware_is_reported_when_ready_without_blocking_when_absent(self):
        for available in (False, True):
            with self.subTest(available=available):

                def read(role, available=available):
                    status = self.status(role)
                    if available and role == "core-agent":
                        status["capabilities"].append({"stream": "host_system_info", "cadence": 3_600_000_000_000})
                    return status

                with patch.object(eudm.time, "sleep") as sleep, patch("builtins.print") as printed:
                    eudm._wait_for_capture_apis(read, COMMIT)
                sleep.assert_not_called()
                output = "\n".join(call.args[0] for call in printed.call_args_list)
                self.assertEqual("Optional host_system_info capture is ready" in output, available)

    def test_unavailable_stream_fails_without_echoing_response(self):
        def read(role):
            value = self.status(role)
            if role == "core-agent":
                value["capabilities"].pop()
            value["private"] = "sensitive-response-body"
            return value

        with patch.object(eudm.time, "monotonic", side_effect=[0, 0, 2]), patch.object(eudm.time, "sleep"):
            with self.assertRaises(Exit) as raised:
                eudm._wait_for_capture_apis(read, COMMIT, timeout=1)
        self.assertIn("core-agent/software", str(raised.exception))
        self.assertNotIn("sensitive-response-body", str(raised.exception))

    def test_malformed_response_is_bounded_by_readiness_timeout(self):
        def read(_role):
            return json.loads("[]")

        with patch.object(eudm.time, "monotonic", side_effect=[0, 0, 2]), patch.object(eudm.time, "sleep"):
            with self.assertRaisesRegex(Exit, "readiness timed out"):
                eudm._wait_for_capture_apis(read, COMMIT, timeout=1)

    def test_previous_capture_protocol_requires_reinstallation(self):
        def read(role):
            value = self.status(role)
            value["protocol_version"] = 1
            return value

        with patch.object(eudm.time, "monotonic", side_effect=[0, 0, 2]), patch.object(eudm.time, "sleep"):
            with self.assertRaises(Exit) as raised:
                eudm._wait_for_capture_apis(read, COMMIT, timeout=1)
        self.assertIn("core-agent", str(raised.exception))
        self.assertIn("process-agent", str(raised.exception))

    def test_legacy_producers_without_inventory_are_not_ready(self):
        def read(role):
            value = self.status(role)
            value["capabilities"] = [
                item for item in value["capabilities"] if item["stream"] not in {"agent_inventory", "host_inventory"}
            ]
            return value

        with patch.object(eudm.time, "monotonic", side_effect=[0, 0, 2]), patch.object(eudm.time, "sleep"):
            with self.assertRaises(Exit) as raised:
                eudm._wait_for_capture_apis(read, COMMIT, timeout=1)
        self.assertIn("core-agent/agent_inventory", str(raised.exception))
        self.assertIn("core-agent/host_inventory", str(raised.exception))

    def test_missing_or_malformed_metric_schedules_require_compatible_producers(self):
        valid = {"family": "cpu", "cadence": 15_000_000_000}
        cases = [
            None,
            [],
            {},
            "sensitive-response-body",
            [None],
            [{}],
            [{"family": "sensitive-response-body", "cadence": 1}],
            [{"family": ["cpu"], "cadence": 1}],
            [valid, valid],
            [valid] * 8,
            *[[{"family": "cpu", "cadence": cadence}] for cadence in (0, -1, True, "15", 1.5, 2**63)],
        ]
        for schedules in cases:
            with self.subTest(schedules=schedules):

                def read(role, schedules=schedules):
                    value = self.status(role)
                    if role == "core-agent":
                        value["capabilities"][0].pop("metric_schedules")
                        if schedules is not None:
                            value["capabilities"][0]["metric_schedules"] = schedules
                    return value

                with patch.object(eudm.time, "monotonic", side_effect=[0, 0, 2]), patch.object(eudm.time, "sleep"):
                    with self.assertRaises(Exit) as raised:
                        eudm._wait_for_capture_apis(read, COMMIT, timeout=1)
                self.assertIn("core-agent/metric check cadences", str(raised.exception))
                self.assertIn("reinstall compatible producers", str(raised.exception))
                self.assertNotIn("sensitive-response-body", str(raised.exception))

    def test_all_fixed_metric_families_with_distinct_cadences_are_ready(self):
        def read(role):
            value = self.status(role)
            if role == "core-agent":
                value["capabilities"][0]["metric_schedules"] = [
                    {"family": family, "cadence": (300 if family == "battery" else 15) * 1_000_000_000}
                    for family in ("cpu", "memory", "disk", "uptime", "wlan", "battery", "network")
                ]
            return value

        with patch.object(eudm.time, "sleep") as sleep, patch("builtins.print"):
            eudm._wait_for_capture_apis(read, COMMIT)
        sleep.assert_not_called()

    def test_waits_for_scheduled_checks_without_substituting_flush_cadence(self):
        calls = 0

        def read(role):
            nonlocal calls
            calls += 1
            value = self.status(role)
            if role == "core-agent" and calls == 1:
                value["capabilities"][0]["metric_schedules"] = []
            return value

        with patch.object(eudm.time, "sleep") as sleep, patch("builtins.print"):
            eudm._wait_for_capture_apis(read, COMMIT)
        self.assertEqual(calls, 4)
        sleep.assert_called_once_with(1)


class TestBuildStamping(unittest.TestCase):
    def test_build_and_output_query_disable_git_stamping(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            outputs = []
            for name in eudm._CAPTURE_TARGETS:
                path = root / "bazel-output" / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.touch()
                outputs.append(str(path.relative_to(root)))
            ctx = MagicMock()
            ctx.run.return_value.stdout = "\n".join(outputs)
            result = eudm._build_capture_binaries(ctx, root, COMMIT, race=True)
            self.assertEqual(result.keys(), eudm._CAPTURE_TARGETS.keys())
            self.assertEqual(len(ctx.run.call_args_list), 2)
            for call, action in zip(ctx.run.call_args_list, ("build", "cquery"), strict=True):
                argv = shlex.split(call.args[0])
                self.assertEqual(argv[:2], ["bazel", action])
                self.assertIn("--nostamp", argv)
                self.assertIn("--workspace_status_command=/usr/bin/true", argv)
                self.assertIn("--lockfile_mode=error", argv)
                self.assertIn("--config=gorace", argv)
                self.assertIn(
                    "--@rules_go//go/config:gc_linkopts=-X=github.com/DataDog/datadog-agent/pkg/version.FullCommit="
                    + COMMIT,
                    argv,
                )
