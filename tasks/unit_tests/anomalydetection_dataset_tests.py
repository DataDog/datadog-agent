import copy
import hashlib
import json
import shutil
import tempfile
import unittest
import zipfile
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from threading import Barrier
from unittest.mock import patch

from tasks.libs.anomalydetection.datasets import DEFAULT_DATASET, _download, prepare_dataset, read_dataset

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

    def test_replay_selection_excludes_removed_and_filtered_rows_but_keeps_cache(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            dataset = self.dataset(directory)
            other = copy.deepcopy(dataset["records"][0])
            other["attributes"]["input"]["scenario"] = "other"
            dataset["records"].append(other)
            all_root, _ = prepare_dataset(dataset, directory / "cache")
            with patch("tasks.libs.anomalydetection.datasets._download", side_effect=AssertionError("cache miss")):
                filtered_root, _ = prepare_dataset(dataset, directory / "cache", scenario="example")
                self.assertEqual([p.name for p in filtered_root.iterdir()], ["example"])
                removed = {**dataset, "records": dataset["records"][:1]}
                current_root, _ = prepare_dataset(removed, directory / "cache")
                self.assertEqual(current_root, filtered_root)
                restored_root, _ = prepare_dataset(dataset, directory / "cache")
                self.assertEqual(restored_root, all_root)
            self.assertEqual({p.name for p in all_root.iterdir()}, {"example", "other"})
            self.assertEqual(
                (filtered_root / "example/parquet/observer-metrics-1.parquet").stat().st_ino,
                (all_root / "example/parquet/observer-metrics-1.parquet").stat().st_ino,
            )

    def test_changed_recording_is_not_hidden_by_scenario_cache(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            first = self.dataset(directory)
            root, _ = prepare_dataset(first, directory / "cache")
            episode = {**EPISODE, "version": 2}
            changed = self.dataset(directory, episode=episode)
            updated_root, _ = prepare_dataset(changed, directory / "cache")
            self.assertEqual(json.loads((updated_root / "example/episode.json").read_text())["version"], 2)
            self.assertEqual(json.loads((root / "example/episode.json").read_text()), EPISODE)

    def test_overlapping_dataset_updates_keep_their_own_recording(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            older = self.dataset(directory)
            cache = directory / "cache"
            older_root, _ = prepare_dataset(older, cache)
            shutil.rmtree(older_root)  # Rebuild the view from the already-verified cache.
            newer = self.dataset(directory, episode={**EPISODE, "version": 2})
            copytree = shutil.copytree
            newer_root = None

            def interleave(source, destination, *args, **kwargs):
                nonlocal newer_root
                if newer_root is None:
                    # Finish the other invocation between cache validation and linking.
                    with patch("tasks.libs.anomalydetection.datasets.shutil.copytree", copytree):
                        newer_root, _ = prepare_dataset(newer, cache)
                return copytree(source, destination, *args, **kwargs)

            with patch("tasks.libs.anomalydetection.datasets.shutil.copytree", side_effect=interleave):
                result, _ = prepare_dataset(older, cache)
            self.assertEqual(result, older_root)
            self.assertEqual(json.loads((result / "example/episode.json").read_text()), EPISODE)
            self.assertEqual(json.loads((newer_root / "example/episode.json").read_text())["version"], 2)

    def test_simultaneous_downloads_publish_one_complete_recording(self):
        from tasks.libs.anomalydetection.datasets import _extract

        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            dataset = self.dataset(directory)
            both_extracted = Barrier(2)

            def extract(*args):
                _extract(*args)
                both_extracted.wait(timeout=5)

            with (
                patch("tasks.libs.anomalydetection.datasets._extract", side_effect=extract),
                ThreadPoolExecutor(max_workers=2) as workers,
            ):
                futures = [workers.submit(prepare_dataset, dataset, directory / "cache") for _ in range(2)]
                results = [future.result(timeout=10) for future in futures]
            self.assertEqual(results[0], results[1])
            root, names = results[0]
            self.assertEqual(names, ["example"])
            self.assertEqual(json.loads((root / "example/episode.json").read_text()), EPISODE)
            self.assertEqual(
                (root / "example/parquet/observer-metrics-1.parquet").read_bytes(), b"original parquet bytes"
            )

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

    def test_resolves_arbitrary_dataset_by_name_or_uuid_in_another_project(self):
        dataset_id = "1372a4b7-3809-4ffb-9ae4-f0bd716940cf"
        for identifier in ("future-recordings", dataset_id):
            pages = [
                [{"id": "another-project", "attributes": {"name": "another-project"}}],
                [{"id": dataset_id, "attributes": {"name": "future-recordings", "current_version": 7}}],
                [{"id": "record", "attributes": {"input": {"scenario": "example"}}}],
            ]
            with (
                self.subTest(identifier=identifier),
                patch("tasks.libs.anomalydetection.datasets._api_pages", side_effect=pages) as request,
            ):
                dataset = read_dataset(identifier, "another-project", "datadoghq.eu")
            self.assertEqual(dataset["id"], dataset_id)
            self.assertEqual(dataset["records"][0]["id"], "record")
            self.assertIn("filter%5Bname%5D=another-project", request.call_args_list[0].args[0])
            self.assertIn(f"/another-project/datasets/{dataset_id}/records?", request.call_args.args[0])
            self.assertIn("filter%5Bversion%5D=7", request.call_args.args[0])
            self.assertEqual(request.call_args.args[1], "datadoghq.eu")
            self.assertNotIn("version", dataset)

    def test_s3_download_supports_named_profiles_and_default_credentials(self):
        for profile in ("", "recording-account"):
            with self.subTest(profile=profile), patch("boto3.Session") as session, patch("boto3.client") as client:
                _download("s3://recordings/nested/episode.zip", Path("recording.zip"), profile)
                if profile:
                    session.assert_called_once_with(profile_name=profile)
                    client.assert_not_called()
                    s3 = session.return_value.client.return_value
                else:
                    session.assert_not_called()
                    client.assert_called_once_with("s3")
                    s3 = client.return_value
                s3.download_file.assert_called_once_with("recordings", "nested/episode.zip", "recording.zip")


class TestLocalEvalCommand(unittest.TestCase):
    dataset = TestDatasetReplay.dataset

    def run_eval(self, directory, *, unordered=False, default=True, fail=False, retain_parquet=False, context=None):
        import shlex
        from types import SimpleNamespace
        from unittest.mock import MagicMock

        from tasks.anomalydetection import eval_scenarios

        dataset = self.dataset(directory)
        context = context if context is not None else MagicMock()

        def run(command, **kwargs):
            args = shlex.split(command)
            if "--headless" in args:
                if unordered and "--retain-parquet" not in args:
                    return SimpleNamespace(failed=True, stderr="timestamps are not globally ordered")
                if fail:
                    return SimpleNamespace(failed=True, stderr="replay crashed")
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
                retain_parquet=retain_parquet,
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

    def test_retained_replay_requires_explicit_selection(self):
        with tempfile.TemporaryDirectory() as temporary:
            result, context, _ = self.run_eval(Path(temporary), unordered=True, retain_parquet=True)
        self.assertEqual(context.run.call_count, 2)  # One replay, then scoring.
        self.assertEqual(result["metadata"]["example"]["replay_mode"], "retained")
        self.assertIn("--retain-parquet", context.run.call_args_list[0].args[0])

    def test_failed_replay_aborts_without_retry_or_publishing_a_partial_score(self):
        from unittest.mock import MagicMock

        from invoke import Exit

        for unordered in (False, True):
            with self.subTest(unordered=unordered), tempfile.TemporaryDirectory() as temporary:
                directory = Path(temporary)
                context = MagicMock()
                error = "not globally ordered" if unordered else "replay crashed"
                with self.assertRaisesRegex(Exit, error):
                    self.run_eval(directory, unordered=unordered, fail=True, context=context)
                context.run.assert_called_once()
                self.assertNotIn("--retain-parquet", context.run.call_args.args[0])
                self.assertFalse((directory / "report.json").exists())

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
                    retain_parquet=True,
                )
            args = shlex.split(ctx.run.call_args.args[0])
        read.assert_called_once_with(DEFAULT_DATASET, "observer-log-ad", "datadoghq.com")
        self.assertIn("config with spaces.json", args)
        self.assertIn("--headless", args)
        self.assertIn("gauge,count", args)
        self.assertIn("--retain-parquet", args)
        self.assertIn(".llmobs/test-dataset", args[args.index("--scenarios-dir") + 1])

    def test_headless_launcher_does_not_retry_unordered_input(self):
        from types import SimpleNamespace
        from unittest.mock import MagicMock

        from invoke import Exit

        from tasks.anomalydetection import launch_testbench

        ctx = MagicMock()
        ctx.run.return_value = SimpleNamespace(failed=True, stderr="not globally ordered")
        with (
            patch("tasks.anomalydetection.read_dataset") as read,
            self.assertRaisesRegex(Exit, "not globally ordered"),
        ):
            launch_testbench.body(ctx, local=True, headless_scenario="example")
        read.assert_not_called()
        ctx.run.assert_called_once()
        self.assertNotIn("--retain-parquet", ctx.run.call_args.args[0])

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
