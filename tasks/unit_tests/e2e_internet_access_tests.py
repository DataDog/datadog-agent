import os
import subprocess
import sys
import tempfile
import unittest
from importlib.util import find_spec
from pathlib import Path, PureWindowsPath
from unittest import mock

from tasks.libs.linter.e2e_internet_access import (
    REGISTRY_PATH,
    SCANNED_PATHSPECS,
    Entry,
    Usage,
    check,
    find_usages,
    in_scope,
    load_registry,
    ls_files_command,
    normalize_paths,
    scan,
    select_scope,
    summarize,
    target_pathspecs,
)

TREE_SITTER_AVAILABLE = find_spec("tree_sitter") is not None and find_spec("tree_sitter_go") is not None
SKIP_REASON = "run with: dda inv --dep tree-sitter==0.26.0 --dep tree-sitter-go==0.25.0 invoke-unit-tests.run"
REPO_ROOT = Path(__file__).resolve().parents[2]


class TestOptionalDependency(unittest.TestCase):
    def test_linter_module_imports_without_tree_sitter(self):
        code = (
            "import sys; sys.modules['tree_sitter'] = None; sys.modules['tree_sitter_go'] = None; import tasks.linter"
        )
        subprocess.run([sys.executable, "-c", code], cwd=REPO_ROOT, check=True)


@unittest.skipUnless(TREE_SITTER_AVAILABLE, SKIP_REASON)
class TestFindUsages(unittest.TestCase):
    def test_function(self):
        src = "package x\n\nfunc TestFoo(t *testing.T) {\n\trun(ec2.WithInternetAccess())\n}\n"
        self.assertEqual(find_usages(src), [(4, "TestFoo")])

    def test_method_receivers(self):
        src = (
            "package x\n"
            "func (s *mySuite) SetupSuite() {\n\tec2.WithInternetAccess()\n}\n"
            "func (mySuite) Other() {\n\tec2.WithInternetAccess()\n}\n"
            "func (s *gen[T]) Generic() {\n\tec2.WithInternetAccess()\n}\n"
        )
        self.assertEqual(find_usages(src), [(3, "mySuite.SetupSuite"), (6, "mySuite.Other"), (9, "gen.Generic")])

    def test_generic_function(self):
        src = "package x\nfunc Run[T any](t *testing.T) {\n\tWithInternetAccess()\n}\n"
        self.assertEqual(find_usages(src), [(3, "Run")])

    def test_multiline_signature(self):
        src = "package x\nfunc TestFoo(\n\tt *testing.T,\n) {\n\tec2.WithInternetAccess()\n}\n"
        self.assertEqual(find_usages(src), [(5, "TestFoo")])

    def test_closure_and_label_attributed_to_top_level_function(self):
        src = (
            "package x\n"
            "func TestFoo(t *testing.T) {\n"
            "outer:\n"
            "\tfor {\n"
            '\t\tt.Run("x", func(t *testing.T) {\n'
            "\t\t\tec2.WithInternetAccess()\n"
            "\t\t})\n"
            "\t\tbreak outer\n"
            "\t}\n"
            "}\n"
        )
        self.assertEqual(find_usages(src), [(6, "TestFoo")])

    def test_package_level_usage_has_no_function(self):
        src = "package x\nvar opts = []ec2.VMOption{\n\tec2.WithInternetAccess(),\n}\n"
        self.assertEqual(find_usages(src), [(3, None)])

    def test_declaration_counts(self):
        src = "package x\nfunc WithInternetAccess() VMOption {\n\treturn nil\n}\n"
        self.assertEqual(find_usages(src), [(2, "WithInternetAccess")])

    def test_function_value_reference(self):
        src = "package x\nfunc F() {\n\topt := ec2.WithInternetAccess\n\t_ = opt\n}\n"
        self.assertEqual(find_usages(src), [(3, "F")])

    def test_comments_strings_and_without_ignored(self):
        src = (
            "package x\n"
            "// WithInternetAccess opts in\n"
            "func TestFoo(t *testing.T) {\n"
            "\t/* WithInternetAccess */\n"
            "\ts := `WithInternetAccess`\n"
            '\tf("WithInternetAccess", s)\n'
            "\tec2.WithoutInternetAccess()\n"
            "}\n"
        )
        self.assertEqual(find_usages(src), [])

    def test_url_string_before_call(self):
        src = 'package x\nfunc F() {\n\tf("curl https://example.com", ec2.WithInternetAccess())\n}\n'
        self.assertEqual(find_usages(src), [(3, "F")])

    def test_multiple_usages_in_one_function(self):
        src = "package x\nfunc F() {\n\tWithInternetAccess()\n\tWithInternetAccess()\n}\n"
        self.assertEqual(find_usages(src), [(3, "F"), (4, "F")])


@unittest.skipUnless(TREE_SITTER_AVAILABLE, SKIP_REASON)
class TestScan(unittest.TestCase):
    def test_scan(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "a.go").write_text("package x\nfunc F() {\n\tWithInternetAccess()\n}\n")
            (root / "b.go").write_text("package x\nfunc G() {}\n")
            self.assertEqual(scan(root, ["a.go", "b.go"]), [Usage("a.go", "F", 3)])


class TestNormalizePaths(unittest.TestCase):
    def test_normalize(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            (root / "test" / "new-e2e").mkdir(parents=True)
            (root / "test" / "new-e2e" / "a_test.go").write_text("package x\n")
            paths = [
                str(root / "test" / "new-e2e" / "a_test.go"),
                str(root / "test" / "new-e2e"),
                str(root / "test" / "new-e2e") + "/",
                str(root),
                str(root / "test" / "new-e2e" / "deleted_test.go"),
                str(root.parent),
                str(root.parent / "elsewhere"),
            ]
            self.assertEqual(
                normalize_paths(paths, root),
                ["test/new-e2e/a_test.go", "test/new-e2e/", "test/new-e2e/", "", "test/new-e2e/deleted_test.go", ""],
            )

    def test_missing_non_go_path_is_a_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            paths = [
                str(root / "test" / "new-e2e" / "tests" / "deleted"),
                str(root / "test" / "new-e2e" / "gone_test.go"),
            ]
            self.assertEqual(normalize_paths(paths, root), ["test/new-e2e/tests/deleted/", "test/new-e2e/gone_test.go"])

    def test_relative_paths_resolve_from_cwd(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            (root / "test" / "new-e2e" / "tests" / "apm").mkdir(parents=True)
            cwd = os.getcwd()
            try:
                os.chdir(root / "test" / "new-e2e")
                self.assertEqual(
                    normalize_paths(["./tests/apm", "tests/x_test.go"], root),
                    ["test/new-e2e/tests/apm/", "test/new-e2e/tests/x_test.go"],
                )
            finally:
                os.chdir(cwd)


class TestSelectScope(unittest.TestCase):
    def test_no_paths_means_full_scan(self):
        self.assertIsNone(select_scope([]))

    def test_registry_change_forces_full_scan(self):
        self.assertIsNone(select_scope(["test/new-e2e/tests/a_test.go", REGISTRY_PATH]))

    def test_directory_containing_all_scanned_dirs_forces_full_scan(self):
        for path in ("", "test/"):
            with self.subTest(path=path):
                self.assertIsNone(select_scope(["test/new-e2e/tests/a_test.go", path]))

    def test_filters_out_of_scope_paths(self):
        paths = [
            "test/new-e2e/tests/a_test.go",
            "test/e2e-framework/b.go",
            "pkg/c.go",
            "test/new-e2e/d.yaml",
            "test/new-e2e/tests/apm/",
            "test/fakeintake/",
            "pkg/",
        ]
        self.assertEqual(
            select_scope(paths), ["test/new-e2e/tests/a_test.go", "test/e2e-framework/b.go", "test/new-e2e/tests/apm/"]
        )

    def test_only_out_of_scope_paths_gives_empty_scope(self):
        self.assertEqual(select_scope(["pkg/c.go"]), [])


class TestTargetPathspecs(unittest.TestCase):
    def test_full_scan(self):
        self.assertEqual(target_pathspecs(None), SCANNED_PATHSPECS)

    def test_scoped(self):
        scope = ["test/new-e2e/tests/a_test.go", "test/new-e2e/tests/apm/"]
        self.assertEqual(target_pathspecs(scope), ("test/new-e2e/tests/a_test.go", "test/new-e2e/tests/apm/*.go"))

    def test_empty_scope(self):
        self.assertEqual(target_pathspecs([]), ())


class TestLsFilesCommand(unittest.TestCase):
    PATHSPECS = ("test/new-e2e/*.go", "test/e2e-framework/*.go")

    @mock.patch("tasks.libs.common.utils.is_windows", return_value=False)
    def test_posix(self, _is_windows):
        self.assertEqual(
            ls_files_command(Path("/src/dd agent"), self.PATHSPECS),
            "git -C '/src/dd agent' ls-files -z -- 'test/new-e2e/*.go' 'test/e2e-framework/*.go'",
        )

    @mock.patch("tasks.libs.common.utils.is_windows", return_value=True)
    def test_windows(self, _is_windows):
        self.assertEqual(
            ls_files_command(PureWindowsPath(r"C:\src\dd agent"), self.PATHSPECS),
            'git -C "C:\\src\\dd agent" ls-files -z -- test/new-e2e/*.go test/e2e-framework/*.go',
        )


class TestInScope(unittest.TestCase):
    def test_in_scope(self):
        scope = ["test/new-e2e/tests/a_test.go", "test/new-e2e/tests/apm/"]
        self.assertTrue(in_scope("test/new-e2e/tests/a_test.go", None))
        self.assertTrue(in_scope("test/new-e2e/tests/a_test.go", scope))
        self.assertTrue(in_scope("test/new-e2e/tests/apm/sub/b_test.go", scope))
        self.assertFalse(in_scope("test/new-e2e/tests/apmx/b_test.go", scope))
        self.assertFalse(in_scope("test/new-e2e/tests/a_test.go.bak", scope))


class TestLoadRegistry(unittest.TestCase):
    def test_valid(self):
        entries, errors = load_registry(
            "entries:\n- file: a.go\n  function: F\n  status: justified\n  reason: Needs it.\n"
        )
        self.assertEqual(errors, [])
        self.assertEqual(entries, [Entry("a.go", "F", "justified", "Needs it.")])

    def test_missing_entries_list(self):
        for text in ("", "foo: 1\n", "entries: foo\n"):
            with self.subTest(text=text):
                entries, errors = load_registry(text)
                self.assertEqual(entries, [])
                self.assertEqual(len(errors), 1)

    def test_invalid_entries(self):
        for case in (
            "- {file: a.go, function: F, status: justified}",
            "- {file: a.go, function: F, status: justified, reason: x, extra: y}",
            "- {file: a.go, function: F, status: justified, reason: '  '}",
            "- {file: a.go, function: F, status: fine, reason: x}",
            "- {file: a.go, function: 3, status: justified, reason: x}",
            "- just-a-string",
        ):
            with self.subTest(case=case):
                entries, errors = load_registry("entries:\n" + case + "\n")
                self.assertEqual(entries, [])
                self.assertEqual(len(errors), 1)

    def test_duplicate_entry(self):
        entries, errors = load_registry(
            "entries:\n"
            "- {file: a.go, function: F, status: justified, reason: x}\n"
            "- {file: a.go, function: F, status: debt, reason: y}\n"
        )
        self.assertEqual(len(entries), 1)
        self.assertEqual(len(errors), 1)
        self.assertIn("duplicate", errors[0])


class TestCheck(unittest.TestCase):
    def test_registered_usages_pass(self):
        usages = [Usage("a.go", "F", 3), Usage("a.go", "F", 7)]
        self.assertEqual(check(usages, [Entry("a.go", "F", "justified", "x")]), [])

    def test_missing_entry_suggests_snippet(self):
        errors = check([Usage("a.go", "F", 3)], [])
        self.assertEqual(len(errors), 1)
        self.assertIn("a.go:3", errors[0])
        self.assertIn("- file: a.go\n  function: F\n", errors[0])

    def test_stale_entry(self):
        errors = check([], [Entry("a.go", "F", "debt", "x")])
        self.assertEqual(len(errors), 1)
        self.assertIn("stale", errors[0])

    def test_renamed_function_reports_missing_and_stale(self):
        errors = check([Usage("a.go", "G", 1)], [Entry("a.go", "F", "justified", "x")])
        self.assertEqual(len(errors), 2)

    def test_same_function_name_in_other_file_is_not_covered(self):
        errors = check([Usage("b.go", "F", 1)], [Entry("a.go", "F", "justified", "x")])
        self.assertEqual(len(errors), 2)

    def test_usage_outside_function(self):
        errors = check([Usage("a.go", None, 2)], [])
        self.assertEqual(len(errors), 1)
        self.assertIn("outside a function", errors[0])

    def test_stale_only_reported_in_scope(self):
        entries = [Entry("a.go", "F", "debt", "x"), Entry("b.go", "G", "debt", "y")]
        errors = check([], entries, scope={"a.go"})
        self.assertEqual(len(errors), 1)
        self.assertIn("a.go F", errors[0])

    def test_partial_scan_still_requires_entry(self):
        errors = check([Usage("a.go", "F", 1)], [Entry("b.go", "G", "debt", "y")], scope={"a.go"})
        self.assertEqual(len(errors), 1)
        self.assertIn("not listed", errors[0])

    def test_directory_scope_catches_deleted_file(self):
        entries = [
            Entry("test/new-e2e/tests/apm/gone_test.go", "F", "debt", "x"),
            Entry("test/other.go", "G", "debt", "y"),
        ]
        errors = check([], entries, scope=["test/new-e2e/tests/apm/"])
        self.assertEqual(len(errors), 1)
        self.assertIn("gone_test.go F", errors[0])


class TestSummarize(unittest.TestCase):
    def test_summarize(self):
        entries = [Entry("a.go", "F", "debt", "x"), Entry("b.go", "G", "untriaged", "y")]
        self.assertEqual(summarize(entries), "2 Internet access exceptions: 0 justified, 1 debt, 1 untriaged")
