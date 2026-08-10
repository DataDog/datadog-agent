import os
import tempfile
import unittest
from unittest import mock

from invoke.exceptions import Exit

from tasks.owners import (
    _discover_smp_leaves,
    _leaf_ownership,
    _team_slugs,
    resolve_run_set_impl,
    smp_inputs_impl,
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


class TestDiscoverSmpLeaves(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.config_dir = os.path.join(self.tmp.name, "test", "regression")
        # leaves (have cases/) + an organizational Dir (logs, no cases/) + an excluded subtree (ebpf)
        for leaf in ("quality_gates", "logs/general", "logs/syslog", "ebpf", "ebpf/config-only"):
            os.makedirs(os.path.join(self.config_dir, leaf, "cases", "some_experiment"))

    def tearDown(self):
        self.tmp.cleanup()

    def test_discovers_leaves_not_dirs(self):
        # `logs` has no cases/ of its own -> not a leaf; experiments under cases/ are not leaves.
        self.assertEqual(
            _discover_smp_leaves(self.config_dir, []),
            ["ebpf", "ebpf/config-only", "logs/general", "logs/syslog", "quality_gates"],
        )

    def test_exclude_drops_subtree(self):
        leaves = _discover_smp_leaves(self.config_dir, ["ebpf"])
        self.assertNotIn("ebpf", leaves)
        self.assertNotIn("ebpf/config-only", leaves)
        self.assertEqual(leaves, ["logs/general", "logs/syslog", "quality_gates"])


class TestLeafOwnershipAndInvolved(unittest.TestCase):
    def setUp(self):
        fd, self.owners_file = tempfile.mkstemp()
        with os.fdopen(fd, "w") as f:
            f.write(CODEOWNERS)

    def tearDown(self):
        os.remove(self.owners_file)

    def test_leaf_ownership(self):
        ownership = _leaf_ownership(
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

    def test_involved_teams_from_changed_files(self):
        involved, _ = smp_inputs_impl(
            # config_dir here only affects discovery, which we don't assert on; ownership uses it too
            # but this test only checks involved_teams from the changed file.
            "test/regression",
            ["test/regression/logs/general/cases/exp/experiment.yaml"],
            ["ebpf"],
            self.owners_file,
        )
        self.assertEqual(involved, ["agent-log-pipelines"])

    def test_no_changed_files_means_no_involved_teams(self):
        involved, _ = smp_inputs_impl("test/regression", [], ["ebpf"], self.owners_file)
        self.assertEqual(involved, [])


class TestResolveRunSet(unittest.TestCase):
    def setUp(self):
        fd, self.owners_file = tempfile.mkstemp()
        with os.fdopen(fd, "w") as f:
            f.write(CODEOWNERS)
        self.tmp = tempfile.TemporaryDirectory()
        self.config_dir = os.path.join(self.tmp.name, "test", "regression")
        for leaf in ("quality_gates", "logs/general"):
            os.makedirs(os.path.join(self.config_dir, leaf, "cases", "exp"))

    def tearDown(self):
        os.remove(self.owners_file)
        self.tmp.cleanup()

    @mock.patch("subprocess.run")
    def test_builds_resolve_command_and_returns_stdout(self, mock_run):
        mock_run.return_value = mock.Mock(returncode=0, stdout="logs/general,quality_gates\n", stderr="")
        out = resolve_run_set_impl(
            self.config_dir,
            "/bin/smp",
            changed_files=["test/regression/logs/general/cases/exp/experiment.yaml"],
            labels="smp/logs/syslog",
            runner="container",
            exclude=["ebpf"],
            owners_file=self.owners_file,
        )
        self.assertEqual(out, "logs/general,quality_gates")
        cmd = mock_run.call_args[0][0]
        self.assertEqual(cmd[:3], ["/bin/smp", "experiments", "resolve"])
        for flag in ("--target-config-dir", "--runner", "--format", "--exclude-path", "--label", "--involved-team"):
            self.assertIn(flag, cmd)
        self.assertIn("path-filter", cmd)
        self.assertIn("ebpf", cmd)
        self.assertIn("smp/logs/syslog", cmd)
        self.assertIn("agent-log-pipelines", cmd)  # involved team resolved from the changed file

    @mock.patch("subprocess.run")
    def test_no_involved_or_labels_omits_those_flags(self, mock_run):
        mock_run.return_value = mock.Mock(returncode=0, stdout="quality_gates\n", stderr="")
        out = resolve_run_set_impl(self.config_dir, "/bin/smp", [], "", "container", ["ebpf"], self.owners_file)
        self.assertEqual(out, "quality_gates")
        cmd = mock_run.call_args[0][0]
        self.assertNotIn("--involved-team", cmd)
        self.assertNotIn("--label", cmd)

    @mock.patch("subprocess.run")
    def test_raises_on_resolve_failure(self, mock_run):
        mock_run.return_value = mock.Mock(returncode=1, stdout="", stderr="boom")
        with self.assertRaises(Exit):
            resolve_run_set_impl(self.config_dir, "/bin/smp", [], "", "container", ["ebpf"], self.owners_file)


if __name__ == "__main__":
    unittest.main()
