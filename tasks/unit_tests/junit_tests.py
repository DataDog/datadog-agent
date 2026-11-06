import json
import shutil
import threading
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from subprocess import CalledProcessError
from unittest.mock import patch

import tasks.libs.common.junit_upload_core as junit
from tasks import junit_tasks
from tasks.libs.owners.parsing import read_owners


class TestFindTarball(unittest.TestCase):
    def test_tarball_in_folder(self):
        tarball_in_folder = "./tasks/unit_tests/testdata/secret.tar.gz"
        self.assertEqual(junit.find_tarball(tarball_in_folder), f"{tarball_in_folder}/secret.tar.gz")

    def test_tarball_in_folder_not_found(self):
        tarball_in_folder = "./tasks/unit_tests/testdata/go_mod_formatter"
        self.assertEqual(junit.find_tarball(tarball_in_folder), f"{tarball_in_folder}/junit.tar.gz")


class TestReadAdditionalTags(unittest.TestCase):
    def test_with_tags(self):
        valid_tags = Path("./tasks/unit_tests/testdata/secret.tar.gz")
        self.assertEqual(
            junit.read_additional_tags(valid_tags),
            [
                "--tags",
                "ci.job.name:kitchen_windows_installer_agent-a6",
                "--tags",
                "arch:x86_64",
                "--tags",
                "os:windows",
                "upload_option.os_version_from_name",
            ],
        )

    def test_without_tags(self):
        invalid_tags = Path("./tasks/unit_tests/testdata")
        self.assertEqual(len(junit.read_additional_tags(invalid_tags)), 0)


class TestSplitJUnitXML(unittest.TestCase):
    def tearDown(self) -> None:
        p = Path("./tasks/unit_tests/testdata/secret.tar.gz")
        for dir in p.iterdir():
            if dir.is_dir():
                shutil.rmtree(dir, ignore_errors=True)

    def test_without_split(self):
        xml_file = Path("./tasks/unit_tests/testdata/secret.tar.gz/bedroom-rspec-win2016-azure-x86_64.xml")
        owners = read_owners("./tasks/unit_tests/testdata/TEST_JUNIT_CODEOWNERS")
        self.assertEqual(junit.split_junitxml(xml_file.parent, xml_file, owners, {}, {}), 1)
        generated_folder = xml_file.parent / "windows-products_base"
        self.assertTrue(generated_folder.exists())

    def test_with_split(self):
        xml_file = Path("./tasks/unit_tests/testdata/secret.tar.gz/-go-src-datadog-agent-junit-out-base.xml")
        owners = read_owners("./tasks/unit_tests/testdata/TEST_JUNIT_CODEOWNERS")
        self.assertEqual(junit.split_junitxml(xml_file.parent, xml_file, owners, {}, {}), 27)


class TestGroupPerTag(unittest.TestCase):
    def test_default_e2e(self):
        test_dir = Path("./tasks/unit_tests/testdata/to_group")
        grouped = junit.group_per_tags(test_dir, [])
        self.assertIn("default", grouped)
        self.assertCountEqual(
            [f"{str(test_dir)}/onepiece", f"{str(test_dir)}/dragonball", f"{str(test_dir)}/gintama"],
            grouped["default"],
        )
        self.assertIn("e2e", grouped)
        self.assertCountEqual([f"{str(test_dir)}/naruto", f"{str(test_dir)}/bleach"], grouped["e2e"])
        self.assertNotIn("kitchen", grouped)
        self.assertNotIn("kitchen-e2e", grouped)


class TestIsE2EInternalFailure(unittest.TestCase):
    def test_e2e_internal_error_string(self):
        xml_file = Path("./tasks/unit_tests/testdata/to_group/naruto")
        self.assertTrue(junit.is_e2e_internal_failure(xml_file))

    def test_fakeintake_timeout(self):
        # Verbatim from the WINA-3079 job log: a fakeintake control endpoint that timed out
        # at the network level, matched on its route prefix.
        xml_file = Path("./tasks/unit_tests/testdata/to_group/bleach")
        self.assertTrue(junit.is_e2e_internal_failure(xml_file))

    def test_unrelated_timeout_is_not_flagged(self):
        # Same dial timeout, but on a non-fakeintake route: it may be a genuine
        # product/networking regression, so it must not be swept in as an infra flake.
        xml_file = Path("./tasks/unit_tests/testdata/to_group/gintama")
        self.assertFalse(junit.is_e2e_internal_failure(xml_file))

    def test_no_failure(self):
        xml_file = Path("./tasks/unit_tests/testdata/to_group/dragonball")
        self.assertFalse(junit.is_e2e_internal_failure(xml_file))


class TestSetTag(unittest.TestCase):
    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1515"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "putsch"})
    def test_default(self):
        tags = junit.set_tags("agent-devx", "base", "", {}, "")
        self.assertEqual(len(tags), 22)
        self.assertIn("slack_channel:agent-devx-ops", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1664"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "beer"})
    def test_flag(self):
        tags = junit.set_tags(
            "agent-devx",
            "base",
            'kitchen-e2e',
            ["upload_option.os_version_from_name"],
            "kitchen-rspec-win2016-azure-x86_64.xml",
        )
        self.assertEqual(len(tags), 26)
        self.assertIn("e2e_internal_error:true", tags)
        self.assertIn("version:win2016", tags)
        self.assertNotIn("upload_option.os_version_from_name", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1789"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "revolution"})
    def test_additional_tags(self):
        tags = junit.set_tags("agent-devx", "base", "", ["--tags", "simple:basique"], "")
        self.assertEqual(len(tags), 24)
        self.assertIn("simple:basique", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1789"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "revolution"})
    def test_additional_tags_from_method(self):
        tags = junit.set_tags(
            "agent-devx", "base", "", junit.read_additional_tags(Path("tasks/unit_tests/testdata")), ""
        )
        self.assertEqual(len(tags), 22)


class TestJUnitUploadFromTGZ(unittest.TestCase):
    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1664"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "beer"})
    @patch("tasks.libs.common.junit_upload_core.check_call")
    @patch("tasks.libs.common.junit_upload_core.which")
    def test_success(self, mock_which, mock_check_call):
        mock_which.side_effect = lambda cmd: f"/usr/local/bin/{cmd}"
        junit.junit_upload_from_tgz(
            "tasks/unit_tests/testdata/testjunit-tests_deb-x64-py3.tgz",
            "tasks/unit_tests/testdata/test_output_no_failure.json",
            "tasks/unit_tests/testdata/TEST_JUNIT_CODEOWNERS",
        )
        mock_check_call.assert_called()
        self.assertEqual(mock_check_call.call_count, 29)
        tmp_dir_vars = ("HOME", "TEMP", "TMP", "TMPDIR")
        seen_tmp_dirs = set()
        for _, kwargs in mock_check_call.call_args_list:
            env = kwargs["env"]
            for k in tmp_dir_vars:
                self.assertIn(k, env)
            last_tmp_dir = env[tmp_dir_vars[-1]]
            self.assertDictEqual({k: env[k] for k in tmp_dir_vars}, dict.fromkeys(tmp_dir_vars, last_tmp_dir))
            self.assertNotIn(last_tmp_dir, seen_tmp_dirs)
            self.assertFalse(Path(last_tmp_dir).exists())
            seen_tmp_dirs.add(last_tmp_dir)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1664"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "beer"})
    @patch("tasks.libs.common.junit_upload_core.check_call")
    @patch("tasks.libs.common.junit_upload_core.which")
    def test_failure(self, mock_which, mock_check_call):
        def raise_on_every_second_call(*args, **kwargs):
            if mock_check_call.call_count % 2 == 0:
                raise CalledProcessError(1, args[0])

        mock_check_call.side_effect = raise_on_every_second_call
        mock_which.side_effect = lambda cmd: f"/usr/local/bin/{cmd}"
        with self.assertRaises(ExceptionGroup) as eg:
            junit.junit_upload_from_tgz(
                "tasks/unit_tests/testdata/testjunit-tests_deb-x64-py3.tgz",
                "tasks/unit_tests/testdata/test_output_no_failure.json",
                "tasks/unit_tests/testdata/TEST_JUNIT_CODEOWNERS",
            )
        mock_check_call.assert_called()
        self.assertEqual(mock_check_call.call_count, 29)
        self.assertEqual(eg.exception.message, "14 junit uploads failed")
        self.assertEqual(len(eg.exception.exceptions), 14)
        for _, kwargs in mock_check_call.call_args_list:
            self.assertFalse(Path(kwargs["env"]["TMPDIR"]).exists())


class TestSkipGitMetadataUploadFlags(unittest.TestCase):
    @patch('tasks.libs.common.junit_upload_core.is_windows', return_value=True)
    def test_flags_on_windows(self, _is_windows):
        self.assertEqual(junit.skip_git_metadata_upload_flags(), ["--skip-git-metadata-upload"])

    @patch('tasks.libs.common.junit_upload_core.is_windows', return_value=False)
    def test_no_flags_on_other_platforms(self, _is_windows):
        self.assertEqual(junit.skip_git_metadata_upload_flags(), [])


class TestUploadJunitxmlsGitMetadataFlag(unittest.TestCase):
    @patch('tasks.libs.common.junit_upload_core._generate_junitxmls')
    @patch('tasks.libs.common.junit_upload_core.check_call')
    @patch('tasks.libs.common.junit_upload_core.which')
    @patch('tasks.libs.common.junit_upload_core.is_windows', return_value=True)
    def test_includes_flag_on_windows(self, _is_windows, mock_which, mock_check_call, mock_generate):
        mock_which.side_effect = lambda cmd: f"/usr/local/bin/{cmd}"
        mock_generate.return_value = iter([(["--tags", "test:flag"], {})])

        with ThreadPoolExecutor() as executor:
            junit._upload_junitxmls([Path("agent-devx_base")], executor)

        args = mock_check_call.call_args[0][0]
        self.assertEqual(args[:4], ["/usr/local/bin/datadog-ci", "junit", "upload", "--skip-git-metadata-upload"])

    @patch('tasks.libs.common.junit_upload_core._generate_junitxmls')
    @patch('tasks.libs.common.junit_upload_core.check_call')
    @patch('tasks.libs.common.junit_upload_core.which')
    @patch('tasks.libs.common.junit_upload_core.is_windows', return_value=False)
    def test_omits_flag_on_other_platforms(self, _is_windows, mock_which, mock_check_call, mock_generate):
        mock_which.side_effect = lambda cmd: f"/usr/local/bin/{cmd}"
        mock_generate.return_value = iter([(["--tags", "test:flag"], {})])

        with ThreadPoolExecutor() as executor:
            junit._upload_junitxmls([Path("agent-devx_base")], executor)

        args = mock_check_call.call_args[0][0]
        self.assertEqual(args[:3], ["/usr/local/bin/datadog-ci", "junit", "upload"])
        self.assertNotIn("--skip-git-metadata-upload", args)


class TestJUnitUploadScheduling(unittest.TestCase):
    @patch("tasks.libs.common.junit_upload_core.check_call")
    def test_invalid_worker_limit_does_not_upload(self, mock_check_call):
        for workers in (0, -1, "two", 1.5):
            with self.subTest(workers=workers), self.assertRaises(ValueError):
                junit.junit_upload_from_tgz("unused.tgz", "unused.json", max_workers=workers)
        mock_check_call.assert_not_called()

    @patch("tasks.junit_tasks.junit_upload_from_tgz")
    def test_task_options(self, upload):
        junit_tasks.junit_upload.body(None, "reports.tgz", "results.json")
        upload.assert_called_once_with("reports.tgz", "results.json", max_workers=None, report_timings=False)
        upload.reset_mock()
        junit_tasks.junit_upload.body(None, "reports.tgz", "results.json", max_workers="4", report_timings=True)
        upload.assert_called_once_with("reports.tgz", "results.json", max_workers=4, report_timings=True)

    @patch.dict(
        "os.environ",
        {"CI_PIPELINE_SOURCE": "push", "CI_JOB_NAME": "fixture", "SYNTHETIC_UPLOAD_SECRET": "fixture-value"},
        clear=True,
    )
    @patch("tasks.libs.common.junit_upload_core.check_call")
    @patch("tasks.libs.common.junit_upload_core.which", return_value="datadog-ci")
    def test_worker_limit_preserves_commands_environment_and_isolation(self, _which, check_call):
        observations = []
        temporary_variables = {"HOME", "TEMP", "TMP", "TMPDIR"}
        for workers in (None, 1, 2):
            check_call.reset_mock()
            junit.junit_upload_from_tgz(
                "tasks/unit_tests/testdata/testjunit-tests_deb-x64-py3.tgz",
                "tasks/unit_tests/testdata/test_output_no_failure.json",
                "tasks/unit_tests/testdata/TEST_JUNIT_CODEOWNERS",
                max_workers=workers,
            )
            normalized = []
            temporary_paths = set()
            for args, kwargs in check_call.call_args_list:
                env = kwargs["env"]
                temporary_paths.add(env["TMPDIR"])
                self.assertEqual({env[key] for key in temporary_variables}, {env["TMPDIR"]})
                self.assertFalse(Path(env["TMPDIR"]).exists())
                # The archive's temporary extraction root differs on each run.
                command = tuple(str(Path(arg).name) if arg.endswith(".xml") else arg for arg in args[0])
                normalized.append((command, sorted((k, v) for k, v in env.items() if k not in temporary_variables)))
            self.assertEqual(len(temporary_paths), 29)
            observations.append(sorted(normalized))
        self.assertEqual(observations[0], observations[1])
        self.assertEqual(observations[0], observations[2])

    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "push"}, clear=True)
    @patch("tasks.libs.common.junit_upload_core._report_upload_timings")
    @patch("tasks.libs.common.junit_upload_core.check_call")
    @patch("tasks.libs.common.junit_upload_core.which", return_value="datadog-ci")
    def test_bounded_workers_drain_all_calls_and_failures(self, _which, check_call, report):
        release = threading.Event()
        at_limit = threading.Event()
        lock = threading.Lock()
        state = {"active": 0, "peak": 0}
        seen = []

        def execute(args, env):
            with lock:
                index = len(seen)
                seen.append(index)
                state["active"] += 1
                state["peak"] = max(state["peak"], state["active"])
                if state["active"] == 2:
                    at_limit.set()
            try:
                if not release.wait(timeout=10):
                    raise AssertionError("test did not release blocked uploads")
                if index in (0, 4):
                    raise CalledProcessError(1, [str(index)])
            finally:
                with lock:
                    state["active"] -= 1

        check_call.side_effect = execute
        with ThreadPoolExecutor(max_workers=1) as controller:
            result = controller.submit(
                junit.junit_upload_from_tgz,
                "tasks/unit_tests/testdata/testjunit-tests_deb-x64-py3.tgz",
                "tasks/unit_tests/testdata/test_output_no_failure.json",
                "tasks/unit_tests/testdata/TEST_JUNIT_CODEOWNERS",
                max_workers=2,
                report_timings=True,
            )
            try:
                self.assertTrue(at_limit.wait(timeout=10), "two uploads should start before either is released")
            finally:
                release.set()
            with self.assertRaises(ExceptionGroup) as errors:
                result.result(timeout=10)
        self.assertEqual(state, {"active": 0, "peak": 2})
        self.assertCountEqual(seen, range(29))
        self.assertEqual(len(errors.exception.exceptions), 2)
        self.assertEqual({int(error.cmd[-1]) for error in errors.exception.exceptions}, {0, 4})
        timings = report.call_args.args[0]
        self.assertEqual(len(timings), 29)
        self.assertEqual(sum(timing["status"] == "failed" for timing in timings), 2)
        for timing in timings:
            self.assertLessEqual(timing["queued"], timing["started"])
            self.assertLessEqual(timing["started"], timing["finished"])


class TestJUnitUploadTimings(unittest.TestCase):
    @patch("tasks.libs.common.junit_upload_core.perf_counter", side_effect=[2.0, 5.0])
    @patch("tasks.libs.common.junit_upload_core._execute_with_partial_isolation")
    def test_failed_upload_records_completion_and_preserves_exception(self, execute, _clock):
        failure = CalledProcessError(7, ["private argument"])
        execute.side_effect = failure
        timing = {"index": 0, "queued": 1.0}
        with self.assertRaises(CalledProcessError) as error:
            junit._execute_timed_upload(["private argument"], {"SECRET": "not logged"}, timing)
        self.assertIs(error.exception, failure)
        self.assertEqual(timing, {"index": 0, "queued": 1.0, "started": 2.0, "finished": 5.0, "status": "failed"})

    @patch("builtins.print")
    def test_summary_separates_queue_run_and_concurrency(self, output):
        junit._report_upload_timings(
            [
                {"index": 0, "queued": 1.0, "started": 2.0, "finished": 5.0, "status": "failed"},
                {"index": 1, "queued": 1.0, "started": 3.0, "finished": 6.0, "status": "success"},
            ]
        )
        summary = json.loads(output.call_args.args[0].removeprefix("JUnit upload timings: "))
        self.assertEqual(summary["upload_count"], 2)
        self.assertEqual(summary["max_active"], 2)
        self.assertEqual(summary["elapsed_seconds"], 5.0)
        self.assertEqual(
            summary["uploads"],
            [
                {"index": 0, "queue_seconds": 1.0, "run_seconds": 3.0, "status": "failed"},
                {"index": 1, "queue_seconds": 2.0, "run_seconds": 3.0, "status": "success"},
            ],
        )

    def test_optional_diagnostics_do_not_raise_when_output_is_closed(self):
        for error in (OSError("closed output"), ValueError("I/O operation on closed file")):
            with self.subTest(error=type(error).__name__), patch("builtins.print", side_effect=error):
                junit._report_upload_timings([])
