import os
import shutil
import tempfile
import unittest
from unittest.mock import MagicMock

from codeowners import CodeOwners

from tasks.libs.owners.linter import (
    ai_artefacts_have_owner,
    codeowner_has_orphans,
    directory_has_packages_without_owner,
    skills_use_agents_directory,
)


class TestCodeownerLinter(unittest.TestCase):
    def setUp(self):
        self.test_dir = tempfile.mkdtemp()
        self.fake_pkgs = ["fake_a", "fake_b", "fake_c"]
        self.pkg_dir = os.path.join(self.test_dir, "pkg")
        self.backup_cwd = os.getcwd()

        # Create pkgs dir
        os.makedirs(self.pkg_dir)
        for pkg in self.fake_pkgs:
            os.makedirs(os.path.join(self.pkg_dir, pkg))

        os.chdir(self.test_dir)

    def tearDown(self):
        shutil.rmtree(self.test_dir)
        os.chdir(self.backup_cwd)

    def test_all_pkg_have_codeowner(self):
        codeowner = CodeOwners("\n".join("/pkg/" + pkg + " @owner" for pkg in self.fake_pkgs))
        self.assertFalse(directory_has_packages_without_owner(codeowner))
        self.assertFalse(codeowner_has_orphans(codeowner))

    def test_pkg_is_missing_codeowner(self):
        codeowner = CodeOwners("\n".join(os.path.join("/pkg/", pkg) + " @owner" for pkg in self.fake_pkgs[:-1]))
        self.assertTrue(directory_has_packages_without_owner(codeowner))
        self.assertFalse(codeowner_has_orphans(codeowner))

    def test_codeowner_rule_is_outdated(self):
        codeowner = CodeOwners(
            "\n".join(os.path.join("/pkg/", pkg) + " @owner" for pkg in [*self.fake_pkgs, "old_deleted_pkg"])
        )
        self.assertFalse(directory_has_packages_without_owner(codeowner))
        self.assertTrue(codeowner_has_orphans(codeowner))

    def test_pkg_owned_by_glob_rule(self):
        # A file only covered by a wildcard rule (no literal /pkg/<name> entry) must still
        # count as owned.
        open(os.path.join(self.pkg_dir, "BUILD.bazel"), "w").close()
        codeowner = CodeOwners(
            "\n".join(["/**/BUILD.bazel @owner", *("/pkg/" + pkg + " @owner" for pkg in self.fake_pkgs)])
        )
        self.assertFalse(directory_has_packages_without_owner(codeowner))
        self.assertFalse(codeowner_has_orphans(codeowner))

    def test_pkg_not_owned_by_blanket_folder_rule(self):
        # A generic /pkg/ rule covers every path under pkg recursively, but it must not be
        # treated as evidence that a specific package has its own dedicated owner.
        codeowner = CodeOwners(
            "\n".join(["/pkg/ @DataDog/agent-runtimes", *("/pkg/" + pkg + " @owner" for pkg in self.fake_pkgs[:-1])])
        )
        self.assertTrue(directory_has_packages_without_owner(codeowner))

    def test_pkg_owned_by_unanchored_glob_rule(self):
        # A rule with no leading '/' (e.g. `BUILD.bazel @owner`) matches after any path
        # separator, not just at the start of the string.
        open(os.path.join(self.pkg_dir, "BUILD.bazel"), "w").close()
        codeowner = CodeOwners(
            "\n".join(["BUILD.bazel @owner", *("/pkg/" + pkg + " @owner" for pkg in self.fake_pkgs)])
        )
        self.assertFalse(directory_has_packages_without_owner(codeowner))

    def test_pkg_not_owned_by_noowner_glob_rule(self):
        # A glob rule with no owners (e.g. a "do not notify anyone" suppression) still matches
        # the path structurally, but must not count as ownership.
        open(os.path.join(self.pkg_dir, "BUILD.bazel"), "w").close()
        codeowner = CodeOwners("\n".join(["/**/BUILD.bazel", *("/pkg/" + pkg + " @owner" for pkg in self.fake_pkgs)]))
        self.assertTrue(directory_has_packages_without_owner(codeowner))


class TestAIArtefactsHaveOwner(unittest.TestCase):
    def setUp(self):
        self.test_dir = tempfile.mkdtemp()
        self.backup_cwd = os.getcwd()
        self._tracked_files = []
        os.chdir(self.test_dir)

    def tearDown(self):
        shutil.rmtree(self.test_dir)
        os.chdir(self.backup_cwd)

    def _create(self, *paths):
        for path in paths:
            full = os.path.join(self.test_dir, path)
            os.makedirs(os.path.dirname(full), exist_ok=True)
            open(full, 'w').close()
            self._tracked_files.append(path)

    def _ctx(self):
        """Return a mock ctx whose run('git ls-files') returns the tracked files."""
        ctx = MagicMock()
        ctx.run.return_value.stdout = "\n".join(self._tracked_files)
        return ctx

    def test_no_ai_artefacts(self):
        # No AGENTS.md, .claude/ or .agents/ — nothing to check
        codeowner = CodeOwners("")
        self.assertFalse(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_agents_md_has_owner(self):
        self._create("pkg/foo/AGENTS.md")
        codeowner = CodeOwners("/pkg/foo/AGENTS.md @DataDog/team-a\n")
        self.assertFalse(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_agents_md_missing_owner(self):
        self._create("pkg/foo/AGENTS.md")
        # A rule for a different path does not cover pkg/foo/AGENTS.md
        codeowner = CodeOwners("/pkg/bar/ @DataDog/team-a\n")
        self.assertTrue(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_agents_file_has_owner(self):
        self._create(".agents/skills/my-skill.md")
        codeowner = CodeOwners("/.agents/ @DataDog/devx\n")
        self.assertFalse(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_agents_file_missing_owner(self):
        self._create(".agents/skills/my-skill.md")
        codeowner = CodeOwners("/pkg/foo/ @DataDog/team-a\n")
        self.assertTrue(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_wildcard_covers_agents_md(self):
        self._create("pkg/foo/AGENTS.md", "comp/bar/AGENTS.md")
        codeowner = CodeOwners("**/AGENTS.md @DataDog/devx\n")
        self.assertFalse(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_mixed_some_missing(self):
        self._create("pkg/foo/AGENTS.md", "pkg/bar/AGENTS.md")
        # Only pkg/foo/AGENTS.md is covered
        codeowner = CodeOwners("/pkg/foo/AGENTS.md @DataDog/team-a\n")
        self.assertTrue(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_catch_all_dot_files_is_not_explicit(self):
        # /.*  matches .agents/ files but is not considered explicit ownership
        self._create(".agents/skills/my-skill.md")
        codeowner = CodeOwners("/.*  @DataDog/agent-devx\n")
        self.assertTrue(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_catch_all_md_is_not_explicit(self):
        # /*.md matches root AGENTS.md but is not considered explicit ownership
        self._create("AGENTS.md")
        codeowner = CodeOwners("/*.md @DataDog/agent-devx\n")
        self.assertTrue(ai_artefacts_have_owner(self._ctx(), codeowner))

    def test_explicit_rule_takes_precedence_over_catch_all(self):
        # Explicit rule listed after the catch-all overrides it (last-match-wins, like GitHub)
        self._create("AGENTS.md")
        codeowner = CodeOwners("/*.md @DataDog/agent-devx\n/AGENTS.md @DataDog/agent-devx\n")
        self.assertFalse(ai_artefacts_have_owner(self._ctx(), codeowner))


class TestSkillsUseAgentsDirectory(unittest.TestCase):
    @staticmethod
    def _ctx(tracked):
        ctx = MagicMock()
        ctx.run.return_value.stdout = "\n".join(tracked)
        return ctx

    def test_symlink_only_is_ok(self):
        # The bare `.claude/skills` symlink with no children is allowed.
        ctx = self._ctx(['.claude/skills', '.agents/skills/foo/SKILL.md'])
        self.assertFalse(skills_use_agents_directory(ctx))

    def test_real_skill_under_claude_is_flagged(self):
        ctx = self._ctx(['.claude/skills/foo/SKILL.md'])
        self.assertTrue(skills_use_agents_directory(ctx))

    def test_skill_under_agents_is_ok(self):
        ctx = self._ctx(['.agents/skills/foo/SKILL.md', '.agents/skills/foo/references/x.md'])
        self.assertFalse(skills_use_agents_directory(ctx))

    def test_nested_claude_skills_is_out_of_scope(self):
        # A nested `.claude/skills` under another directory falls outside the root-scoped check.
        ctx = self._ctx(['test/new-e2e/x/.claude/skills/y/SKILL.md'])
        self.assertFalse(skills_use_agents_directory(ctx))
