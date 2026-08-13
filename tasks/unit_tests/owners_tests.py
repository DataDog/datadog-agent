import json
import os
import tempfile
import unittest
from unittest import mock

from invoke.exceptions import Exit

from tasks.owners import (
    _discover_folders,
    _folder_ownership,
    _team_slugs,
    resolve_run_set_impl,
    smp_inputs_impl,
    smp_pr_context_impl,
)

# CODEOWNERS with repo-relative patterns; SMP owns the tree, log-pipelines owns logs/ exclusively.
CODEOWNERS = """\
/test/regression/ @DataDog/single-machine-performance
/test/regression/ebpf @DataDog/single-machine-performance @DataDog/ebpf-platform
/test/regression/logs/ @DataDog/agent-log-pipelines
"""


class TestTeamSlugs(unittest.TestCase):
    def test_normalizes_and_filters_to_teams(self):
        self.assertEqual(
            _team_slugs(["@DataDog/Agent-Log-Pipelines", "@DataDog/single-machine-performance"]),
            ["agent-log-pipelines", "single-machine-performance"],
        )

    def test_drops_non_datadog_owners(self):
        self.assertEqual(_team_slugs(["@someuser", "@DataDog/team-a"]), ["team-a"])


class TestDiscoverFolders(unittest.TestCase):
    """Folders come from `smp experiments list` (single discovery source), derived from each
    experiment path's segment above `cases/`."""

    @mock.patch("subprocess.run")
    def test_derives_folders_from_experiment_paths(self, mock_run):
        listing = [
            {"path": "quality_gates/cases/quality_gate_idle"},
            {"path": "quality_gates/cases/quality_gate_logs"},
            {"path": "logs/general/cases/logs_general"},
            {"path": "logs/syslog/cases/logs_syslog_1"},
        ]
        mock_run.return_value = mock.Mock(returncode=0, stdout=json.dumps(listing), stderr="")
        folders = _discover_folders("test/regression", "selection.yaml", "/bin/smp", ["ebpf"])
        self.assertEqual(folders, ["logs/general", "logs/syslog", "quality_gates"])
        cmd = mock_run.call_args[0][0]
        self.assertEqual(cmd[:3], ["/bin/smp", "experiments", "list"])
        for flag in ("--target-config-dir", "--manifest", "--format", "--exclude-path"):
            self.assertIn(flag, cmd)
        self.assertIn("selection.yaml", cmd)
        self.assertIn("ebpf", cmd)

    @mock.patch("subprocess.run")
    def test_raises_on_list_failure(self, mock_run):
        mock_run.return_value = mock.Mock(returncode=1, stdout="", stderr="boom")
        with self.assertRaises(Exit):
            _discover_folders("test/regression", "selection.yaml", "/bin/smp", [])


class TestFolderOwnershipAndInvolved(unittest.TestCase):
    def setUp(self):
        fd, self.owners_file = tempfile.mkstemp()
        with os.fdopen(fd, "w") as f:
            f.write(CODEOWNERS)

    def tearDown(self):
        os.remove(self.owners_file)

    def test_folder_ownership(self):
        ownership = _folder_ownership(
            ["quality_gates", "logs/general", "logs/syslog"], "test/regression", self.owners_file
        )
        self.assertEqual(
            ownership,
            {
                "quality_gates": ["single-machine-performance"],
                "logs/general": ["agent-log-pipelines"],
                "logs/syslog": ["agent-log-pipelines"],
            },
        )

    @mock.patch("tasks.owners._discover_folders", return_value=[])
    def test_involved_teams_from_changed_files(self, _folders):
        involved, _ = smp_inputs_impl(
            "test/regression",
            "selection.yaml",
            "/bin/smp",
            ["test/regression/logs/general/cases/exp/experiment.yaml"],
            ["ebpf"],
            self.owners_file,
        )
        self.assertEqual(involved, ["agent-log-pipelines"])

    @mock.patch("tasks.owners._discover_folders", return_value=[])
    def test_no_changed_files_means_no_involved_teams(self, _folders):
        involved, _ = smp_inputs_impl("test/regression", "selection.yaml", "/bin/smp", [], ["ebpf"], self.owners_file)
        self.assertEqual(involved, [])


class TestResolveRunSet(unittest.TestCase):
    OWNERS_FILE = ".github/CODEOWNERS"  # unused (smp_inputs_impl is mocked), kept for signature clarity

    @mock.patch(
        "tasks.owners.smp_inputs_impl",
        return_value=(["agent-log-pipelines"], {"logs/general": ["agent-log-pipelines"]}),
    )
    @mock.patch("subprocess.run")
    def test_builds_resolve_command_and_returns_stdout(self, mock_run, _inputs):
        mock_run.return_value = mock.Mock(
            returncode=0,
            stdout="logs/general/cases/logs_general,quality_gates/cases/quality_gate_idle\n",
            stderr="",
        )
        out = resolve_run_set_impl(
            "test/regression",
            "selection.yaml",
            "/bin/smp",
            changed_files=["test/regression/logs/general/cases/exp/experiment.yaml"],
            labels="smp/logs/syslog",
            runner="container",
            exclude=["ebpf"],
            owners_file=self.OWNERS_FILE,
        )
        self.assertEqual(out, "logs/general/cases/logs_general,quality_gates/cases/quality_gate_idle")
        cmd = mock_run.call_args[0][0]
        self.assertEqual(cmd[:3], ["/bin/smp", "experiments", "resolve"])
        for flag in (
            "--target-config-dir",
            "--manifest",
            "--runner",
            "--format",
            "--exclude-path",
            "--label",
            "--involved-team",
        ):
            self.assertIn(flag, cmd)
        self.assertIn("path-filter", cmd)
        self.assertIn("selection.yaml", cmd)
        self.assertIn("ebpf", cmd)
        self.assertIn("smp/logs/syslog", cmd)
        self.assertIn("agent-log-pipelines", cmd)

    @mock.patch("tasks.owners.smp_inputs_impl", return_value=([], {}))
    @mock.patch("subprocess.run")
    def test_no_involved_or_labels_omits_those_flags(self, mock_run, _inputs):
        mock_run.return_value = mock.Mock(returncode=0, stdout="quality_gates/cases/quality_gate_idle\n", stderr="")
        out = resolve_run_set_impl(
            "test/regression", "selection.yaml", "/bin/smp", [], "", "container", ["ebpf"], self.OWNERS_FILE
        )
        self.assertEqual(out, "quality_gates/cases/quality_gate_idle")
        cmd = mock_run.call_args[0][0]
        self.assertNotIn("--involved-team", cmd)
        self.assertNotIn("--label", cmd)

    @mock.patch("tasks.owners.smp_inputs_impl", return_value=([], {}))
    @mock.patch("subprocess.run")
    def test_raises_on_resolve_failure(self, mock_run, _inputs):
        mock_run.return_value = mock.Mock(returncode=1, stdout="", stderr="boom")
        with self.assertRaises(Exit):
            resolve_run_set_impl(
                "test/regression", "selection.yaml", "/bin/smp", [], "", "container", ["ebpf"], self.OWNERS_FILE
            )


class TestSmpPrContext(unittest.TestCase):
    @mock.patch("tasks.libs.ciproviders.github_api.GithubAPI")
    def test_returns_labels_and_files_for_open_pr(self, mock_gh_cls):
        gh = mock_gh_cls.return_value
        gh.get_pr_for_branch.return_value = [mock.Mock(number=42)]
        gh.get_pr_labels.return_value = ["smp/logs/syslog", "team/agent-log-pipelines"]
        gh.get_pr_files.return_value = ["test/regression/logs/general/cases/exp/experiment.yaml", "tasks/owners.py"]
        labels, files = smp_pr_context_impl("mybranch")
        self.assertEqual(labels, "smp/logs/syslog,team/agent-log-pipelines")
        self.assertEqual(files, ["test/regression/logs/general/cases/exp/experiment.yaml", "tasks/owners.py"])
        gh.get_pr_for_branch.assert_called_once_with(head_branch_name="mybranch")

    @mock.patch("tasks.libs.ciproviders.github_api.GithubAPI")
    def test_no_open_pr_returns_empty(self, mock_gh_cls):
        mock_gh_cls.return_value.get_pr_for_branch.return_value = []
        labels, files = smp_pr_context_impl("mybranch")
        self.assertEqual(labels, "")
        self.assertEqual(files, [])


if __name__ == "__main__":
    unittest.main()
