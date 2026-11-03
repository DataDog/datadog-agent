import hashlib
import json
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import patch

from tasks.libs.anomalydetection.datasets import DEFAULT_DATASET, _read_dataset, prepare_dataset

EPISODE = {
    "start_time": "2026-01-01T00:00:00Z",
    "end_time": "2026-01-01T00:35:00Z",
    "baseline": {"start": "2026-01-01T00:00:00Z", "end": "2026-01-01T00:05:00Z"},
    "disruption": {"start": "2026-01-01T00:05:00Z", "end": "2026-01-01T00:30:00Z"},
    "cooldown": {"start": "2026-01-01T00:30:00Z", "end": "2026-01-01T00:35:00Z"},
}


class TestDatasetReplay(unittest.TestCase):
    def dataset(self, directory, *, episode=EPISODE, members=None):
        archive = directory / "source.zip"
        with zipfile.ZipFile(archive, "w") as output:
            output.writestr("tmp/gensim-archive/results/episode.json", json.dumps(episode))
            output.writestr("tmp/gensim-archive/parquet/observer-metrics-1.parquet", b"original parquet bytes")
            for name, content in (members or {}).items():
                output.writestr(name, content)
        return {
            "id": "test-dataset",
            "name": "Recordings",
            "records": [
                {
                    "attributes": {
                        "input": {
                            "scenario": "example",
                            "recording": {
                                "uri": archive.as_uri(),
                                "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
                            },
                        }
                    }
                }
            ],
        }

    def test_stages_original_files_and_reuses_cache_without_download(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            dataset = self.dataset(directory)
            root, names = prepare_dataset(dataset, directory / "cache")
            self.assertEqual(names, ["example"])
            self.assertEqual(json.loads((root / "example/episode.json").read_text()), EPISODE)
            self.assertEqual(
                (root / "example/parquet/observer-metrics-1.parquet").read_bytes(), b"original parquet bytes"
            )
            with patch("tasks.libs.anomalydetection.datasets._download", side_effect=AssertionError("cache miss")):
                self.assertEqual(prepare_dataset(dataset, directory / "cache"), (root, names))

    def test_changed_recording_is_not_hidden_by_scenario_cache(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            first = self.dataset(directory)
            root, _ = prepare_dataset(first, directory / "cache")
            episode = {**EPISODE, "version": 2}
            changed = self.dataset(directory, episode=episode)
            prepare_dataset(changed, directory / "cache")
            self.assertEqual(json.loads((root / "example/episode.json").read_text())["version"], 2)

    def test_rejects_bad_checksum_without_destroying_existing_cache(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            dataset = self.dataset(directory)
            root, _ = prepare_dataset(dataset, directory / "cache")
            dataset["records"][0]["attributes"]["input"]["recording"]["sha256"] = "0" * 64
            with self.assertRaisesRegex(ValueError, "SHA-256"):
                prepare_dataset(dataset, directory / "cache")
            self.assertTrue((root / "example/episode.json").is_file())

    def test_rejects_path_traversal_in_archives_and_scenario_names(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            dataset = self.dataset(directory, members={"../escape": "bad"})
            with self.assertRaisesRegex(ValueError, "Unsafe"):
                prepare_dataset(dataset, directory / "cache")
            dataset["records"][0]["attributes"]["input"]["scenario"] = "../escape"
            with self.assertRaisesRegex(ValueError, "Invalid scenario"):
                prepare_dataset(dataset, directory / "cache")

    def test_requires_unambiguous_episode_member(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            dataset = self.dataset(directory, members={"tmp/gensim-archive/results/second.json": json.dumps(EPISODE)})
            with self.assertRaisesRegex(ValueError, "episode_member"):
                prepare_dataset(dataset, directory / "cache")
            dataset["records"][0]["attributes"]["input"]["recording"]["episode_member"] = (
                "tmp/gensim-archive/results/episode.json"
            )
            prepare_dataset(dataset, directory / "cache")

    def test_resolves_latest_without_exposing_a_version_selector(self):
        pages = [
            [{"id": "project", "attributes": {"name": "observer-log-ad"}}],
            [{"id": "dataset", "attributes": {"name": "Recordings", "current_version": 7}}],
            [{"id": "record", "attributes": {"input": {"scenario": "example"}}}],
        ]
        with patch("tasks.libs.anomalydetection.datasets._api_pages", side_effect=pages) as request:
            dataset = _read_dataset("Recordings", "observer-log-ad", "datadoghq.com")
        self.assertEqual(dataset["records"][0]["id"], "record")
        self.assertIn("filter%5Bversion%5D=7", request.call_args.args[0])
        self.assertNotIn("version", dataset)


class TestLocalEvalCommand(unittest.TestCase):
    dataset = TestDatasetReplay.dataset

    def run_eval(self, directory, *, unordered=False, default=True):
        import shlex
        from types import SimpleNamespace
        from unittest.mock import MagicMock

        from tasks.anomalydetection import eval_scenarios

        dataset = self.dataset(directory)
        context = MagicMock()

        def run(command, **kwargs):
            args = shlex.split(command)
            if "--headless" in args:
                if unordered and "--retain-parquet" not in args:
                    return SimpleNamespace(failed=True, stderr="timestamps are not globally ordered")
                output = Path(args[args.index("--output") + 1])
                output.write_text(json.dumps({"metadata": {"scenario": "example"}}))
                return SimpleNamespace(failed=False)
            score = {
                "f1": 0.5,
                "precision": 0.5,
                "recall": 0.5,
                "num_predictions": 1,
                "num_baseline_fps": 0,
                "num_filtered_warmup": 0,
                "num_filtered_cascading": 0,
            }
            return SimpleNamespace(failed=False, stdout=json.dumps(score), stderr="")

        context.run.side_effect = run
        kwargs = {} if default else {"dataset": "Recordings"}
        with patch("tasks.anomalydetection.read_dataset", return_value=dataset) as read:
            result = eval_scenarios.body(
                context,
                build=False,
                scenarios_dir=str(directory / "cache"),
                scenario_output_dir=str(directory / "outputs"),
                main_report_path=str(directory / "report.json"),
                **kwargs,
            )
        return result, context, read

    def test_default_dataset_preserves_f1_and_records_local_inspect_command(self):
        with tempfile.TemporaryDirectory() as temporary:
            result, context, read = self.run_eval(Path(temporary))
        read.assert_called_once_with(DEFAULT_DATASET, "observer-log-ad", "datadoghq.com")
        self.assertEqual(result["score"], 0.5)
        row = result["metadata"]["example"]
        self.assertEqual(row["f1"], 0.5)
        self.assertNotIn("false_positives", row)
        self.assertIn("--local", row["inspect_command"])
        self.assertIn("--scenario example", row["inspect_command"])
        self.assertIn(".llmobs/test-dataset", row["inspect_command"])
        self.assertNotIn("--score-no-faults", context.run.call_args.args[0])

    def test_explicit_dataset_overrides_default(self):
        with tempfile.TemporaryDirectory() as temporary:
            _, _, read = self.run_eval(Path(temporary), default=False)
        read.assert_called_once_with("Recordings", "observer-log-ad", "datadoghq.com")

    def test_unordered_input_retries_sorted_and_records_replay_mode(self):
        with tempfile.TemporaryDirectory() as temporary:
            result, context, _ = self.run_eval(Path(temporary), unordered=True)
        self.assertEqual(context.run.call_count, 3)
        self.assertEqual(result["metadata"]["example"]["replay_mode"], "retained")
        self.assertIn("--retain-parquet", context.run.call_args_list[1].args[0])

    def test_local_eval_never_reads_llmobs(self):
        from unittest.mock import MagicMock

        from invoke import Exit

        from tasks.anomalydetection import eval_scenarios

        with patch("tasks.anomalydetection.read_dataset") as read, self.assertRaisesRegex(Exit, "Specify --scenario"):
            eval_scenarios.body(MagicMock(), local=True, build=False)
        read.assert_not_called()

    def test_headless_launcher_stages_default_dataset_and_preserves_flags(self):
        import shlex
        from types import SimpleNamespace
        from unittest.mock import MagicMock

        from tasks.anomalydetection import launch_testbench

        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            dataset = self.dataset(directory)
            ctx = MagicMock()
            ctx.run.return_value = SimpleNamespace(failed=False)
            with patch("tasks.anomalydetection.read_dataset", return_value=dataset) as read:
                launch_testbench.body(
                    ctx,
                    headless_scenario="example",
                    scenarios_dir=str(directory / "cache"),
                    config="config with spaces.json",
                    allowed_types="gauge,count",
                )
            args = shlex.split(ctx.run.call_args.args[0])
        read.assert_called_once_with(DEFAULT_DATASET, "observer-log-ad", "datadoghq.com")
        self.assertIn("config with spaces.json", args)
        self.assertIn("--headless", args)
        self.assertIn("gauge,count", args)
        self.assertIn(".llmobs/test-dataset", args[args.index("--scenarios-dir") + 1])

    def test_headless_launcher_retries_unordered_input(self):
        from types import SimpleNamespace
        from unittest.mock import MagicMock

        from tasks.anomalydetection import launch_testbench

        ctx = MagicMock()
        ctx.run.side_effect = [
            SimpleNamespace(failed=True, stderr="not globally ordered"),
            SimpleNamespace(failed=False),
        ]
        with patch("tasks.anomalydetection.read_dataset") as read:
            launch_testbench.body(ctx, local=True, headless_scenario="example")
        read.assert_not_called()
        self.assertEqual(ctx.run.call_count, 2)
        self.assertIn("--retain-parquet", ctx.run.call_args.args[0])

    def test_interactive_launcher_uses_same_staged_recording_and_stops_its_process(self):
        import shlex
        from unittest.mock import MagicMock

        from tasks.anomalydetection import launch_testbench

        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            dataset = self.dataset(directory)
            ctx = MagicMock()
            with (
                patch("tasks.anomalydetection.read_dataset", return_value=dataset),
                patch("tasks.anomalydetection.socket.socket"),
                patch("tasks.anomalydetection.subprocess.Popen") as spawn,
            ):
                launch_testbench.body(
                    ctx,
                    scenarios_dir=str(directory / "cache"),
                    scenario="example",
                    api_port=18087,
                    ui_port=15177,
                    only="bocpd,anomaly_scorer",
                )
            args = spawn.call_args.args[0]
        self.assertIn("127.0.0.1:18087", args)
        self.assertIn(".llmobs/test-dataset", args[args.index("--scenarios-dir") + 1])
        self.assertIn("15177", shlex.split(ctx.run.call_args.args[0]))
        env = ctx.run.call_args.kwargs["env"]
        self.assertEqual(env["OBSERVER_TESTBENCH_API_URL"], "http://127.0.0.1:18087")
        self.assertEqual(env["VITE_SCENARIO"], "example")
        spawn.return_value.terminate.assert_called_once()
        spawn.return_value.wait.assert_called_once_with(timeout=5)


if __name__ == "__main__":
    unittest.main()
