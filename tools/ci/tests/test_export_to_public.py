"""Exercise the export script against local repositories; no network or credentials needed.

Run: python3 -m unittest discover -s tools/ci/tests -p 'test_export_to_public.py' -v
"""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "export-to-public.sh"


class ExportToPublicTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="export-test-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.public = self.root / "public"
        self.internal = self.root / "internal"
        self.clone = self.root / "clone"
        self.work = self.root / "export"
        self.env = os.environ | {
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_AUTHOR_NAME": "Export test",
            "GIT_AUTHOR_EMAIL": "export@example.invalid",
            "GIT_COMMITTER_NAME": "Export test",
            "GIT_COMMITTER_EMAIL": "export@example.invalid",
            "SOURCE_URL": str(self.public),
            "DESTINATION_URL": str(self.internal),
            "GIT_EDITOR": "true",
        }
        self.git(self.root, "init", "-b", "main", str(self.public))
        self.write(self.public, ".github/CODEOWNERS", "* @DataDog/agent-devx\n")
        self.write(self.public, "shared", "base\n")
        self.commit(self.public, "base")
        # Internal main: public main, an internal CI commit, then a sync merge of newer public work.
        self.git(self.root, "clone", "--no-hardlinks", str(self.public), str(self.internal))
        self.write(self.internal, ".github/CODEOWNERS", "* @ddoghq/agent-devx\n")
        self.write(self.internal, "ci.yml", "project: ddoghq/datadog-agent\n")
        self.commit(self.internal, "internal CI adaptation")
        self.write(self.public, "upstream-file", "upstream change\n")
        self.commit(self.public, "upstream change")
        self.git(self.internal, "pull", "--no-rebase", "--no-edit", str(self.public), "main")
        # A developer clone with a feature branch on internal main.
        self.git(self.root, "clone", "--no-hardlinks", str(self.internal), str(self.clone))
        self.git(self.clone, "switch", "-c", "feature")
        self.write(self.clone, "feature", "fix\n")
        self.commit(self.clone, "Fix the thing")

    def git(self, cwd, *args):
        p = subprocess.run(["git", "-C", str(cwd), *args], env=self.env, text=True, capture_output=True)
        if p.returncode:
            self.fail(p.stderr)
        return p.stdout.strip()

    @staticmethod
    def write(repo, name, content):
        path = repo / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def commit(self, repo, message):
        self.git(repo, "add", ".")
        self.git(repo, "commit", "-m", message)

    def export(self, *args, success=True, cwd=None):
        p = subprocess.run(
            ["bash", str(SCRIPT), *args],
            cwd=cwd or self.clone,
            env=self.env,
            text=True,
            capture_output=True,
        )
        self.assertEqual(p.returncode == 0, success, p.stdout + p.stderr)
        return p.stderr

    def exported(self):
        return self.git(self.work, "log", "--format=%s", "refs/export-to-public/upstream..HEAD").splitlines()

    def test_exports_only_own_commits_on_public_main(self):
        out = self.export("--workdir", str(self.work), "feature")
        self.assertIn("Ready to export", out)
        self.assertIn("HEAD:refs/heads/feature", out)
        self.assertEqual(self.exported(), ["Fix the thing"])
        self.assertEqual(self.git(self.work, "show", "HEAD:.github/CODEOWNERS"), "* @DataDog/agent-devx")
        self.assertNotIn("cherry picked", self.git(self.work, "log", "-1", "--format=%B"))

    def test_runs_from_a_public_clone(self):
        public_clone = self.root / "public-clone"
        self.git(self.root, "clone", "--no-hardlinks", str(self.public), str(public_clone))
        self.git(public_clone, "fetch", str(self.clone), "feature:refs/private/feature")
        out = self.export("--workdir", str(self.work), "refs/private/feature", cwd=public_clone)
        self.assertIn("HEAD:refs/heads/feature", out)
        self.assertEqual(self.exported(), ["Fix the thing"])

    def test_merging_internal_main_into_the_branch_is_ignored(self):
        self.write(self.internal, "internal-only", "x\n")
        self.commit(self.internal, "another internal commit")
        self.git(self.clone, "pull", "--no-rebase", "--no-edit", str(self.internal), "main")
        self.export("--workdir", str(self.work), "feature")
        self.assertEqual(self.exported(), ["Fix the thing"])

    def test_never_pushes(self):
        before = self.git(self.public, "for-each-ref")
        self.export("--workdir", str(self.work), "--name", "public-fix", "feature")
        self.assertEqual(self.git(self.public, "for-each-ref"), before)

    def test_internal_reference_in_content_blocks(self):
        self.write(self.clone, "feature", "see ddoghq/datadog-agent#12\n")
        self.commit(self.clone, "Mention the private PR")
        out = self.export("--workdir", str(self.work), "feature", success=False)
        self.assertIn("feature: see ddoghq/datadog-agent#12", out)

    def test_reference_added_then_removed_still_blocks(self):
        # The final diff is clean, but the first commit would still be published.
        self.write(self.clone, "feature", "see ddoghq/datadog-agent#12\n")
        self.commit(self.clone, "Mention the private PR")
        self.write(self.clone, "feature", "fix\n")
        self.commit(self.clone, "Drop the mention")
        out = self.export("--workdir", str(self.work), "feature", success=False)
        self.assertIn("feature: see ddoghq/datadog-agent#12", out)

    def test_internal_file_added_then_removed_still_blocks(self):
        self.write(self.clone, ".github/chainguard/policy.sts.yaml", "subject: x\n")
        self.commit(self.clone, "Add a policy")
        self.git(self.clone, "rm", "-q", ".github/chainguard/policy.sts.yaml")
        self.git(self.clone, "commit", "-m", "Remove the policy")
        out = self.export("--workdir", str(self.work), "feature", success=False)
        self.assertIn("Internal-only file changed: .github/chainguard/policy.sts.yaml", out)

    def test_internal_author_identity_blocks(self):
        self.write(self.clone, "other", "y\n")
        self.git(self.clone, "add", ".")
        self.git(self.clone, "commit", "-m", "Other fix", "--author", "jane-doe_ddog <jane@example.invalid>")
        out = self.export("--workdir", str(self.work), "feature", success=False)
        self.assertIn("Commit identities use internal accounts", out)
        self.assertNotIn("Commit messages reference", out)

    def test_internal_gitlab_project_id_blocks(self):
        self.write(self.clone, "feature", 'project_id: "14012"\n')
        self.commit(self.clone, "Point to the project")
        out = self.export("--workdir", str(self.work), "feature", success=False)
        self.assertIn('feature: project_id: "14012"', out)

    def test_change_already_public_under_another_sha_is_dropped(self):
        self.write(self.clone, "other", "same\n")
        self.commit(self.clone, "Land upstream too")
        self.write(self.public, "other", "same\n")
        self.commit(self.public, "Same change, landed publicly")
        self.export("--workdir", str(self.work), "feature")
        self.assertEqual(self.exported(), ["Fix the thing"])

    def test_internal_reference_in_message_blocks(self):
        self.write(self.clone, "other", "y\n")
        self.commit(self.clone, "Follow-up of ddoghq/datadog-agent#12")
        out = self.export("--workdir", str(self.work), "feature", success=False)
        self.assertIn("Commit messages", out)

    def test_internal_path_needs_allow_path(self):
        self.write(self.clone, ".github/chainguard/policy.sts.yaml", "subject: x\n")
        self.commit(self.clone, "Add a policy")
        out = self.export("--workdir", str(self.work), "feature", success=False)
        self.assertIn("Internal-only file changed: .github/chainguard/policy.sts.yaml", out)
        self.export("--check", "--workdir", str(self.work), "--allow-path", ".github/chainguard/*")
        # The allowance is remembered for later checks in the same worktree.
        self.export("--check", "--workdir", str(self.work))

    def test_conflict_stops_then_check_after_resolution(self):
        self.write(self.clone, "ci.yml", "project: ddoghq/datadog-agent\nnew: job\n")
        self.commit(self.clone, "Add a CI job")
        out = self.export("--workdir", str(self.work), "feature", success=False)
        self.assertIn("git cherry-pick --continue", out)
        self.export("--check", "--workdir", str(self.work), success=False)  # still in progress
        self.write(self.work, "ci.yml", "new: job\n")
        self.git(self.work, "add", "ci.yml")
        self.git(self.work, "cherry-pick", "--continue")
        self.export("--check", "--workdir", str(self.work))
        self.assertEqual(self.exported(), ["Add a CI job", "Fix the thing"])

    def test_branch_already_in_internal_main_is_refused(self):
        out = self.export("--workdir", str(self.work), "origin/main", success=False)
        self.assertIn("no commit", out)


if __name__ == "__main__":
    unittest.main()
