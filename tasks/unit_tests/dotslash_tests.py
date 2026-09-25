import copy
import hashlib
import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import threading
import unittest
import zipfile
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from unittest.mock import patch

# Direct loading keeps these tests independent of the Invoke task collection.
ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("dotslash_policy", ROOT / "tasks/libs/linter/dotslash.py")
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)


def document(name="buildifier"):
    return json.loads((ROOT / "tools/bin" / name).read_text(encoding="utf-8").split("\n", 1)[1])


class TestDiscovery(unittest.TestCase):
    def test_only_official_headers_identify_manifests(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            directory = root / "tools/bin"
            directory.mkdir(parents=True)
            contents = {
                "lf": policy.SHEBANG + b"\n{}",
                "crlf": policy.SHEBANG + b"\r\n{}",
                "unusual.extension": policy.SHEBANG + b"\n{}",
                "mise-tool": b"#!/usr/bin/env mise\n{}",
                "shell": b"#!/bin/sh\necho test",
                "binary.exe": b"MZ\x00\xff",
                "prefix": policy.SHEBANG + b"-extra\n{}",
                "arguments": policy.SHEBANG + b" --flag\n{}",
                "bom": b"\xef\xbb\xbf" + policy.SHEBANG + b"\n{}",
                "unterminated": policy.SHEBANG,
            }
            for name, content in contents.items():
                (directory / name).write_bytes(content)
            self.assertEqual([p.name for p in policy.discover(root)], ["crlf", "lf", "unusual.extension"])


class TestPolicy(unittest.TestCase):
    def test_current_manifests(self):
        for name in ("buildifier", "vault"):
            with self.subTest(name=name):
                policy.validate_document(document(name), name)

    def test_artifact_errors(self):
        cases = (
            ("size", 0),
            ("hash", "blake3"),
            ("providers_order", "weighted-random"),
            ("path", "C:/tool.exe"),
            ("providers", []),
            ("providers", [{}]),
            ("providers", ["https://example.com/tool"]),
            ("providers", [{"type": "other", "url": "https://example.com/tool"}]),
            ("providers", [{"url": "http://example.com/tool"}]),
            ("providers", [{"url": "https://user:secret@example.com/tool"}]),
        )
        for key, value in cases:
            with self.subTest(key=key, value=value):
                data = document()
                data["platforms"]["linux-x86_64"][key] = value
                with self.assertRaises(ValueError):
                    policy.validate_document(data, "buildifier")

    def test_missing_platform_and_wrong_name(self):
        data = document()
        with self.assertRaises(ValueError):
            policy.validate_document(data, "different")
        del data["platforms"]["linux-aarch64"]
        with self.assertRaises(ValueError):
            policy.validate_document(data, "buildifier")

    def test_provider_order_and_platform_mapping(self):
        data = document()
        data["platforms"]["linux-x86_64"]["providers"].reverse()
        with self.assertRaises(ValueError):
            policy.validate_document(data, "buildifier")
        data = document()
        data["platforms"]["linux-x86_64"] = data["platforms"]["linux-aarch64"]
        with self.assertRaises(ValueError):
            policy.validate_document(data, "buildifier")

    def test_consistent_versions_without_duplicated_pins(self):
        data = document()
        for provider in data["platforms"]["linux-x86_64"]["providers"]:
            prefix, asset = provider["url"].rsplit("/", 1)
            provider["url"] = prefix.rsplit("/", 1)[0] + "/v0.0.0/" + asset
        with self.assertRaisesRegex(ValueError, "same release"):
            policy.validate_document(data, "buildifier")

    def test_unconfigured_tools_need_no_probe_registry(self):
        data = document()
        data["name"] = "another-tool"
        del data["metadata"]
        policy.validate_document(data, "another-tool")


class TestNativeConfiguration(unittest.TestCase):
    def test_absent_or_empty_configuration_never_invents_a_command(self):
        for metadata in ({}, {"native_check": {}}, {"native_check": {"pattern": "example"}}):
            data = {"name": "example", "metadata": metadata}
            self.assertIsNone(policy.native_check(data))
        self.assertIsNone(policy.native_check({"name": "example"}))

    def test_invalid_configuration(self):
        cases = [
            None,
            [],
            {"commmand": ["example"]},
            {"command": ["example"]},
            {"command": "example --help", "pattern": "help"},
            {"command": [], "pattern": "help"},
            {"command": ["different"], "pattern": "help"},
            {"command": ["example", 1], "pattern": "help"},
            {"command": ["example", "\0"], "pattern": "help"},
            {"command": ["example"], "pattern": ""},
            {"command": ["example"], "pattern": "["},
        ]
        for check in cases:
            with self.subTest(check=check), self.assertRaises(ValueError):
                policy.native_check({"name": "example", "metadata": {"native_check": check}})

    def test_checkout_launcher_and_literal_arguments_on_both_platforms(self):
        manifest = ROOT / "tools/bin/example"
        arguments = ["example", "a space", "%PATH%", "&", ""]
        check = policy.native_check(
            {"name": "example", "metadata": {"native_check": {"command": arguments, "pattern": "help"}}}
        )
        for host, suffix in (("linux-x86_64", ""), ("windows-x86_64", ".exe")):
            with self.subTest(host=host), patch.object(policy, "run", return_value="help") as run:
                policy.execute_check(manifest, check, host, ROOT / ".cache/test", ROOT)
                self.assertEqual(run.call_args.args[0], [str(manifest) + suffix, *arguments[1:]])
                self.assertTrue(run.call_args.kwargs["combined"])

    def test_output_mismatch(self):
        check = policy.native_check(
            {"name": "example", "metadata": {"native_check": {"command": ["example"], "pattern": "expected"}}}
        )
        with patch.object(policy, "run", return_value="wrong"), self.assertRaisesRegex(ValueError, "did not match"):
            policy.execute_check(ROOT / "example", check, "linux-x86_64", ROOT, ROOT)

    def test_real_process_matches_stdout_and_stderr_and_rejects_nonzero(self):
        for stream in ("stdout", "stderr"):
            output = policy.run(
                [sys.executable, "-c", f"import sys; print('expected', file=sys.{stream})"], combined=True
            )
            self.assertRegex(output, "expected")
        with self.assertRaisesRegex(ValueError, "exited with 2"):
            policy.run([sys.executable, "-c", "print('expected'); raise SystemExit(2)"], combined=True)

    def test_process_timeout_is_bounded(self):
        with patch.object(policy.subprocess, "run", side_effect=subprocess.TimeoutExpired("example", 300)) as run:
            with self.assertRaises(subprocess.TimeoutExpired):
                policy.run(["example"])
            self.assertEqual(run.call_args.kwargs["timeout"], 300)


class TestSelection(unittest.TestCase):
    def select(self, changes, untracked=""):
        paths = [ROOT / "tools/bin/buildifier", ROOT / "tools/bin/vault"]
        with patch.object(policy, "run", side_effect=["base\n", changes, untracked]):
            return policy.selected_manifests(ROOT, paths, "origin/main")

    def test_manifest_and_untracked_selection(self):
        self.assertEqual(self.select("tools/bin/vault\0"), [ROOT / "tools/bin/vault"])
        self.assertEqual(self.select("", "tools/bin/buildifier\0"), [ROOT / "tools/bin/buildifier"])
        self.assertEqual(self.select("docs/example.md\0"), [])

    def test_shared_inputs_and_shims_select_everything(self):
        for change in (*policy.VALIDATOR_INPUTS, "tools/bin/vault.exe"):
            with self.subTest(change=change):
                self.assertEqual(len(self.select(change)), 2)

    def test_missing_history_does_not_skip_validation(self):
        with patch.object(policy, "run", side_effect=ValueError("Missing ref")), self.assertRaises(ValueError):
            policy.selected_manifests(ROOT, [], "missing")


class TestFetch(unittest.TestCase):
    def test_missing_and_outside_executables_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            outside = root / "outside"
            outside.write_text("fixture", encoding="utf-8")
            for path in (outside, root / "cache/missing", root):
                with self.subTest(path=path), patch.object(policy, "run", return_value=str(path)):
                    with self.assertRaisesRegex(ValueError, "regular executable"):
                        policy.fetch("dotslash", root / "manifest", root / "cache")

    def test_provider_isolation_and_environment_preservation(self):
        entry = document()["platforms"]["linux-x86_64"]
        original = copy.deepcopy(entry)
        snapshots = []
        caches = []

        def fetch(_interpreter, manifest, cache):
            self.assertFalse(cache.exists())
            snapshots.append(json.loads(manifest.read_text(encoding="utf-8").split("\n", 1)[1]))
            caches.append(cache)
            return cache / "tool"

        with patch.dict(os.environ, {"DOTSLASH_CACHE": "original"}), patch.object(policy, "fetch", side_effect=fetch):
            for provider in entry["providers"]:
                policy.verify_download("dotslash", "example", entry, provider, "windows-x86_64")
            self.assertEqual(os.environ["DOTSLASH_CACHE"], "original")
        self.assertEqual(entry, original)
        self.assertEqual(len(caches), len(entry["providers"]))
        self.assertEqual(len(set(caches)), len(caches))
        for snapshot, provider, cache in zip(snapshots, entry["providers"], caches, strict=True):
            self.assertEqual(snapshot["platforms"], {"windows-x86_64": {**entry, "providers": [provider]}})
            self.assertFalse(cache.parent.exists())


class TestReporting(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / "tools/bin").mkdir(parents=True)
        (self.root / "mise.toml").write_bytes((ROOT / "mise.toml").read_bytes())
        for name in ("buildifier", "vault"):
            for suffix in ("", ".exe"):
                (self.root / "tools/bin" / (name + suffix)).write_bytes(
                    (ROOT / "tools/bin" / (name + suffix)).read_bytes()
                )
            (self.root / "tools/bin" / name).chmod(0o755)
        self.addCleanup(patch.stopall)
        patch.object(policy.shutil, "which", return_value="dotslash").start()
        patch.object(policy, "run", side_effect=self.run_process).start()

    def run_process(self, argv, **_kwargs):
        if argv[1:] == ["--version"]:
            pin = policy.tomllib.loads((self.root / "mise.toml").read_text(encoding="utf-8"))["tools"]["dotslash"][
                "version"
            ]
            return f"DotSlash {pin}\n"
        if argv[1:3] == ["--", "parse"]:
            return Path(argv[3]).read_text(encoding="utf-8").split("\n", 1)[1]
        if argv[1:3] == ["ls-files", "--stage"]:
            return "100755 unused 0\ttools/bin/buildifier\0" + "100755 unused 0\ttools/bin/vault\0"
        raise AssertionError(f"Unexpected process: {argv}")

    def report(self):
        return json.loads((self.root / "report.json").read_text(encoding="utf-8"))

    def test_metadata_controls_all_native_execution(self):
        data = document("vault")
        del data["metadata"]
        policy.write_manifest(self.root / "tools/bin/vault", data)
        with patch.object(policy, "execute_check") as execute:
            self.assertTrue(policy.validate(self.root, smoke=True, report="report.json"))
        self.assertEqual(execute.call_count, 2)
        self.assertTrue(all(call.args[0].name == "buildifier" for call in execute.call_args_list))
        self.assertEqual(execute.call_args_list[0].args[3], execute.call_args_list[1].args[3])
        self.assertEqual(self.report()["results"][-1]["status"], "skipped")

    def test_invalid_regex_is_reported_and_prevents_execution(self):
        data = document()
        data["metadata"]["native_check"]["pattern"] = "["
        policy.write_manifest(self.root / "tools/bin/buildifier", data)
        with patch.object(policy, "execute_check") as execute:
            self.assertFalse(policy.validate(self.root, smoke=True, report="report.json"))
        execute.assert_not_called()
        self.assertIn("Invalid native-check pattern", self.report()["results"][1]["message"])
        self.assertEqual(self.report()["results"][2]["status"], "passed")

    def test_failed_provider_does_not_hide_other_results_or_allow_execution(self):
        expected = sum(
            len(entry["providers"])
            for name in ("buildifier", "vault")
            for entry in document(name)["platforms"].values()
        )
        with (
            patch.object(
                policy, "verify_download", side_effect=[ValueError("Unavailable")] + [None] * (expected - 1)
            ) as download,
            patch.object(policy, "execute_check") as execute,
        ):
            self.assertFalse(policy.validate(self.root, download=True, smoke=True, report="report.json"))
        self.assertEqual(download.call_count, expected)
        execute.assert_not_called()
        self.assertEqual(sum(result["status"] == "failed" for result in self.report()["results"]), 1)
        self.assertEqual(sum(result["stage"] == "download" for result in self.report()["results"]), expected)

    def test_failed_native_cold_check_skips_warm_check_but_continues_other_tools(self):
        with patch.object(policy, "execute_check", side_effect=[ValueError("Broken"), None, None]) as execute:
            self.assertFalse(policy.validate(self.root, smoke=True, report="report.json"))
        self.assertEqual([call.args[0].name for call in execute.call_args_list], ["buildifier", "vault", "vault"])

    def test_bad_companion_is_a_policy_failure(self):
        (self.root / "tools/bin/vault.exe").write_bytes(b"incorrect")
        self.assertFalse(policy.validate(self.root, report="report.json"))
        self.assertIn("approved generic", self.report()["results"][-1]["message"])

    def test_runner_architecture_is_checked(self):
        with patch.dict(os.environ, {"DOTSLASH_EXPECTED_PLATFORM": "incorrect-platform"}):
            self.assertFalse(policy.validate(self.root, report="report.json"))
        self.assertIn("must execute natively", self.report()["results"][0]["message"])

    def test_setup_failure_still_writes_report(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(policy.shutil, "which", return_value=None):
            root = Path(temporary)
            self.assertFalse(policy.validate(root, report="results/report.json"))
            report = json.loads((root / "results/report.json").read_text(encoding="utf-8"))
            self.assertFalse(report["success"])
            self.assertEqual(report["results"][0]["stage"], "setup")


@contextmanager
def artifact_server(blobs):
    """Serve tiny artifacts and record requests, returning HTTP 503 for missing paths."""
    requests = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            requests.append(self.path)
            if self.path not in blobs:
                self.send_error(503)
                return
            blob = blobs[self.path]
            self.send_response(200)
            self.send_header("Content-Length", str(len(blob)))
            self.end_headers()
            self.wfile.write(blob)

        def log_message(self, *_args):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", requests
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


@unittest.skipUnless(
    os.environ.get("DOTSLASH_TEST_EXECUTABLE"), "Set DOTSLASH_TEST_EXECUTABLE to test the pinned interpreter."
)
class TestInterpreterIntegration(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.interpreter = os.environ["DOTSLASH_TEST_EXECUTABLE"]
        pin = policy.tomllib.loads((ROOT / "mise.toml").read_text(encoding="utf-8"))["tools"]["dotslash"]["version"]
        if policy.run([cls.interpreter, "--version"]).strip() != f"DotSlash {pin}":
            raise AssertionError("Integration tests require the interpreter pinned by mise.toml.")

    def test_parser_enforces_schema(self):
        cases = (
            ("size", True),
            ("size", -1),
            ("size", "1"),
            ("digest", "A" * 64),
            ("digest", "0" * 63),
            ("digest", int("1" * 64)),
            ("format", "tgz"),
            ("format", []),
            ("path", "../tool"),
            ("path", "/tool"),
            ("path", "bin\\tool"),
            ("path", "bin//tool"),
            ("path", "tool\0"),
            ("path", "."),
            ("path", ""),
            ("path", 1),
            ("providers", {}),
        )
        with tempfile.TemporaryDirectory() as temporary:
            manifest = Path(temporary) / "example"
            for key, value in cases:
                with self.subTest(key=key, value=value):
                    data = document()
                    data["platforms"]["linux-x86_64"][key] = value
                    policy.write_manifest(manifest, data)
                    with self.assertRaises(ValueError):
                        policy.run([self.interpreter, "--", "parse", str(manifest)])

    def test_parser_preserves_metadata_and_accepts_jsonc(self):
        data = document()
        data["metadata"]["provenance"] = {"example": "preserved"}
        with tempfile.TemporaryDirectory() as temporary:
            manifest = Path(temporary) / "example"
            manifest.write_bytes(
                policy.SHEBANG
                + b"\n// Comments and trailing commas are valid.\n"
                + json.dumps(data)[:-1].encode()
                + b",}"
            )
            parsed = json.loads(policy.run([self.interpreter, "--", "parse", str(manifest)]))
        self.assertEqual(parsed, data)

    def test_repository_policy_rejects_values_accepted_by_parser(self):
        with tempfile.TemporaryDirectory() as temporary:
            manifest = Path(temporary) / "example"
            for key, value in (("size", 0), ("hash", "blake3"), ("providers", []), ("providers", [{}])):
                with self.subTest(key=key, value=value):
                    data = document()
                    data["platforms"]["linux-x86_64"][key] = value
                    policy.write_manifest(manifest, data)
                    parsed = json.loads(policy.run([self.interpreter, "--", "parse", str(manifest)]))
                    with self.assertRaises(ValueError):
                        policy.validate_document(parsed, "buildifier")

    def fixtures(self):
        contents = b"This fixture must never be executed.\n"
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, "w") as output:
            output.writestr("bin/foreign-tool", contents)
        return ((contents, {}), (archive.getvalue(), {"format": "zip"}))

    def test_verified_downloads_and_missing_archive_member(self):
        for blob, options in self.fixtures():
            with self.subTest(options=options), artifact_server({"/artifact": blob}) as (base, _requests):
                provider = {"url": base + "/artifact"}
                entry = {
                    "size": len(blob),
                    "hash": "sha256",
                    "digest": hashlib.sha256(blob).hexdigest(),
                    "path": "bin/foreign-tool",
                    "providers": [provider],
                    **options,
                }
                host = policy.host_platform()
                policy.verify_download(self.interpreter, "example", entry, provider, host)
                invalid = [("size", len(blob) + 1), ("digest", "0" * 64)]
                if options:
                    invalid.append(("path", "missing"))
                for field, value in invalid:
                    with self.subTest(field=field), self.assertRaises(ValueError):
                        policy.verify_download(self.interpreter, "example", {**entry, field: value}, provider, host)

    def test_cache_reuse_and_provider_fallback(self):
        for blob, options in self.fixtures():
            with (
                self.subTest(options=options),
                artifact_server({"/artifact": blob}) as (base, requests),
                tempfile.TemporaryDirectory() as temporary,
            ):
                root = Path(temporary)
                manifest = root / "example"
                entry = {
                    "size": len(blob),
                    "hash": "sha256",
                    "digest": hashlib.sha256(blob).hexdigest(),
                    "path": "bin/foreign-tool",
                    "providers": [{"url": base + "/unavailable"}, {"url": base + "/artifact"}],
                    **options,
                }
                data = {"name": "example", "platforms": {policy.host_platform(): entry}}
                policy.write_manifest(manifest, data)
                executable = policy.fetch(self.interpreter, manifest, root / "cache")
                self.assertGreaterEqual(len(requests), 2)
                self.assertTrue(all(path == "/unavailable" for path in requests[:-1]))
                self.assertEqual(requests[-1], "/artifact")
                requests.clear()
                entry["providers"] = [{"url": base + "/unavailable"}]
                policy.write_manifest(manifest, data)
                self.assertEqual(policy.fetch(self.interpreter, manifest, root / "cache"), executable)
                self.assertEqual(requests, [])


if __name__ == "__main__":
    unittest.main()
