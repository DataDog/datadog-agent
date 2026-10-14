import os
import subprocess
import sys
import tempfile
import unittest
from importlib.util import find_spec
from pathlib import Path

from tasks.libs.linter.e2e_internet_access import (
    REGISTRY_PATH,
    REPO_ROOT,
    Entry,
    Registry,
    Usage,
    normalize_paths,
    scan,
    select_scope,
)

TREE_SITTER_AVAILABLE = find_spec("tree_sitter") is not None and find_spec("tree_sitter_go") is not None
SKIP_REASON = "run with: dda inv --dep tree-sitter==0.26.0 --dep tree-sitter-go==0.25.0 invoke-unit-tests.run"
GO_FILE = Path("test/new-e2e/x_test.go")


def write_files(root: Path, files: dict[str, str]) -> None:
    for name, content in files.items():
        (root / name).parent.mkdir(parents=True, exist_ok=True)
        (root / name).write_text(content)


def usages_in(source: str) -> list[tuple[int, str | None]]:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        write_files(root, {str(GO_FILE): source})
        return [(usage.line, usage.function) for usage in scan(root, None)]


class TestOptionalDependency(unittest.TestCase):
    def test_linter_module_imports_without_tree_sitter(self):
        code = (
            "import sys; sys.modules['tree_sitter'] = None; sys.modules['tree_sitter_go'] = None; import tasks.linter"
        )
        subprocess.run([sys.executable, "-c", code], cwd=REPO_ROOT, check=True)


class TestRepoRoot(unittest.TestCase):
    def test_repo_root(self):
        self.assertTrue((REPO_ROOT / REGISTRY_PATH.parent).is_dir())


@unittest.skipUnless(TREE_SITTER_AVAILABLE, SKIP_REASON)
class TestFindUsages(unittest.TestCase):
    def test_function(self):
        src = "package x\n\nfunc TestFoo(t *testing.T) {\n\trun(ec2.WithInternetAccess())\n}\n"
        self.assertEqual(usages_in(src), [(4, "TestFoo")])

    def test_method_receivers(self):
        src = (
            "package x\n"
            "func (s *mySuite) SetupSuite() {\n\tec2.WithInternetAccess()\n}\n"
            "func (mySuite) Other() {\n\tec2.WithInternetAccess()\n}\n"
            "func (s *gen[T]) Generic() {\n\tec2.WithInternetAccess()\n}\n"
        )
        self.assertEqual(usages_in(src), [(3, "mySuite.SetupSuite"), (6, "mySuite.Other"), (9, "gen.Generic")])

    def test_generic_function(self):
        src = "package x\nfunc Run[T any](t *testing.T) {\n\tWithInternetAccess()\n}\n"
        self.assertEqual(usages_in(src), [(3, "Run")])

    def test_multiline_signature(self):
        src = "package x\nfunc TestFoo(\n\tt *testing.T,\n) {\n\tec2.WithInternetAccess()\n}\n"
        self.assertEqual(usages_in(src), [(5, "TestFoo")])

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
        self.assertEqual(usages_in(src), [(6, "TestFoo")])

    def test_package_level_usage_has_no_function(self):
        src = "package x\nvar opts = []ec2.VMOption{\n\tec2.WithInternetAccess(),\n}\n"
        self.assertEqual(usages_in(src), [(3, None)])

    def test_declaration_counts(self):
        src = "package x\nfunc WithInternetAccess() VMOption {\n\treturn nil\n}\n"
        self.assertEqual(usages_in(src), [(2, "WithInternetAccess")])

    def test_function_value_reference(self):
        src = "package x\nfunc F() {\n\topt := ec2.WithInternetAccess\n\t_ = opt\n}\n"
        self.assertEqual(usages_in(src), [(3, "F")])

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
        self.assertEqual(usages_in(src), [])

    def test_url_string_before_call(self):
        src = 'package x\nfunc F() {\n\tf("curl https://example.com", ec2.WithInternetAccess())\n}\n'
        self.assertEqual(usages_in(src), [(3, "F")])

    def test_multiple_usages_in_one_function(self):
        src = "package x\nfunc F() {\n\tWithInternetAccess()\n\tWithInternetAccess()\n}\n"
        self.assertEqual(usages_in(src), [(3, "F"), (4, "F")])


@unittest.skipUnless(TREE_SITTER_AVAILABLE, SKIP_REASON)
class TestScan(unittest.TestCase):
    SOURCE = "package x\nfunc F() {\n\tWithInternetAccess()\n}\n"

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)
        write_files(
            self.root,
            {
                "test/new-e2e/tests/apm/a_test.go": self.SOURCE,
                "test/new-e2e/tests/apm/sub/b_test.go": self.SOURCE,
                "test/new-e2e/tests/other/c_test.go": self.SOURCE,
                "test/new-e2e/tests/other/d_test.go": "package x\nfunc G() {}\n",
                "test/e2e-framework/e.go": self.SOURCE,
                "test/new-e2e/tests/apm/notes.md": "WithInternetAccess",
                "pkg/f.go": self.SOURCE,
            },
        )

    def files(self, scope):
        return sorted(usage.file.as_posix() for usage in scan(self.root, scope))

    def test_full_scan(self):
        self.assertEqual(
            self.files(None),
            [
                "test/e2e-framework/e.go",
                "test/new-e2e/tests/apm/a_test.go",
                "test/new-e2e/tests/apm/sub/b_test.go",
                "test/new-e2e/tests/other/c_test.go",
            ],
        )

    def test_scoped_scan(self):
        scope = [Path("test/new-e2e/tests/apm"), Path("test/new-e2e/tests/other/c_test.go")]
        self.assertEqual(
            self.files(scope),
            [
                "test/new-e2e/tests/apm/a_test.go",
                "test/new-e2e/tests/apm/sub/b_test.go",
                "test/new-e2e/tests/other/c_test.go",
            ],
        )

    def test_overlapping_and_missing_scope(self):
        scope = [
            Path("test/new-e2e/tests/apm"),
            Path("test/new-e2e/tests/apm/a_test.go"),
            Path("test/new-e2e/tests/deleted"),
            Path("test/new-e2e/tests/gone_test.go"),
        ]
        self.assertEqual(
            self.files(scope), ["test/new-e2e/tests/apm/a_test.go", "test/new-e2e/tests/apm/sub/b_test.go"]
        )


class TestNormalizePaths(unittest.TestCase):
    def test_normalize(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            write_files(root, {"test/new-e2e/a_test.go": "package x\n"})
            paths = [
                str(root / "test" / "new-e2e" / "a_test.go"),
                str(root / "test" / "new-e2e") + "/",
                str(root),
                str(root / "test" / "new-e2e" / "deleted"),
                str(root.parent),
                str(root.parent / "elsewhere"),
            ]
            self.assertEqual(
                normalize_paths(paths, root),
                [
                    Path("test/new-e2e/a_test.go"),
                    Path("test/new-e2e"),
                    Path("."),
                    Path("test/new-e2e/deleted"),
                    Path("."),
                ],
            )

    def test_relative_paths_resolve_from_cwd(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            (root / "test" / "new-e2e" / "tests" / "apm").mkdir(parents=True)
            cwd = os.getcwd()
            try:
                os.chdir(root / "test" / "new-e2e")
                self.assertEqual(
                    normalize_paths(["./tests/apm", "tests/x_test.go"], root),
                    [Path("test/new-e2e/tests/apm"), Path("test/new-e2e/tests/x_test.go")],
                )
            finally:
                os.chdir(cwd)


class TestSelectScope(unittest.TestCase):
    def test_no_paths_means_full_scan(self):
        self.assertIsNone(select_scope([]))

    def test_registry_change_forces_full_scan(self):
        self.assertIsNone(select_scope([Path("test/new-e2e/tests/a_test.go"), REGISTRY_PATH]))

    def test_directory_containing_all_scanned_dirs_forces_full_scan(self):
        for path in (Path("."), Path("test")):
            with self.subTest(path=path):
                self.assertIsNone(select_scope([Path("test/new-e2e/tests/a_test.go"), path]))

    def test_filters_out_of_scope_paths(self):
        paths = [
            Path("test/new-e2e/tests/a_test.go"),
            Path("test/e2e-framework/b.go"),
            Path("pkg/c.go"),
            Path("test/new-e2e/tests/apm"),
            Path("test/new-e2e"),
            Path("test/fakeintake"),
            Path("pkg"),
        ]
        self.assertEqual(
            select_scope(paths),
            [
                Path("test/new-e2e/tests/a_test.go"),
                Path("test/e2e-framework/b.go"),
                Path("test/new-e2e/tests/apm"),
                Path("test/new-e2e"),
            ],
        )

    def test_only_out_of_scope_paths_gives_empty_scope(self):
        self.assertEqual(select_scope([Path("pkg/c.go")]), [])


class TestRegistryParse(unittest.TestCase):
    def test_valid(self):
        registry = Registry.parse("entries:\n- file: a.go\n  function: F\n  status: justified\n  reason: Needs it.\n")
        self.assertEqual(registry.errors, [])
        self.assertEqual(registry.entries, [Entry(Path("a.go"), "F", "justified", "Needs it.")])

    def test_missing_entries_list(self):
        for text in ("", "foo: 1\n", "entries: foo\n"):
            with self.subTest(text=text):
                registry = Registry.parse(text)
                self.assertEqual(registry.entries, [])
                self.assertEqual(len(registry.errors), 1)

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
                registry = Registry.parse("entries:\n" + case + "\n")
                self.assertEqual(registry.entries, [])
                self.assertEqual(len(registry.errors), 1)

    def test_duplicate_entry(self):
        registry = Registry.parse(
            "entries:\n"
            "- {file: a.go, function: F, status: justified, reason: x}\n"
            "- {file: ./a.go, function: F, status: debt, reason: y}\n"
        )
        self.assertEqual(len(registry.entries), 1)
        self.assertEqual(len(registry.errors), 1)
        self.assertIn("duplicate", registry.errors[0])


class TestRegistryCheck(unittest.TestCase):
    @staticmethod
    def check(usages, entries, scope=None):
        return Registry(entries).check(usages, scope)

    def test_registered_usages_pass(self):
        usages = [Usage(Path("a.go"), "F", 3), Usage(Path("a.go"), "F", 7)]
        self.assertEqual(self.check(usages, [Entry(Path("a.go"), "F", "justified", "x")]), [])

    def test_missing_entry_suggests_snippet(self):
        errors = self.check([Usage(Path("a.go"), "F", 3)], [])
        self.assertEqual(len(errors), 1)
        self.assertIn("a.go:3", errors[0])
        self.assertIn("- file: a.go\n  function: F\n", errors[0])

    def test_stale_entry(self):
        errors = self.check([], [Entry(Path("a.go"), "F", "debt", "x")])
        self.assertEqual(len(errors), 1)
        self.assertIn("stale", errors[0])

    def test_renamed_function_reports_missing_and_stale(self):
        errors = self.check([Usage(Path("a.go"), "G", 1)], [Entry(Path("a.go"), "F", "justified", "x")])
        self.assertEqual(len(errors), 2)

    def test_same_function_name_in_other_file_is_not_covered(self):
        errors = self.check([Usage(Path("b.go"), "F", 1)], [Entry(Path("a.go"), "F", "justified", "x")])
        self.assertEqual(len(errors), 2)

    def test_usage_outside_function(self):
        errors = self.check([Usage(Path("a.go"), None, 2)], [])
        self.assertEqual(len(errors), 1)
        self.assertIn("outside a function", errors[0])

    def test_stale_only_reported_in_scope(self):
        entries = [Entry(Path("a.go"), "F", "debt", "x"), Entry(Path("b.go"), "G", "debt", "y")]
        errors = self.check([], entries, scope=[Path("a.go")])
        self.assertEqual(len(errors), 1)
        self.assertIn("a.go F", errors[0])

    def test_partial_scan_still_requires_entry(self):
        errors = self.check(
            [Usage(Path("a.go"), "F", 1)], [Entry(Path("b.go"), "G", "debt", "y")], scope=[Path("a.go")]
        )
        self.assertEqual(len(errors), 1)
        self.assertIn("not listed", errors[0])

    def test_directory_scope_catches_deleted_file(self):
        entries = [
            Entry(Path("test/new-e2e/tests/apm/gone_test.go"), "F", "debt", "x"),
            Entry(Path("test/new-e2e/tests/apmx/other_test.go"), "G", "debt", "y"),
        ]
        errors = self.check([], entries, scope=[Path("test/new-e2e/tests/apm")])
        self.assertEqual(len(errors), 1)
        self.assertIn("gone_test.go F", errors[0])

    def test_file_scope_does_not_match_lookalike(self):
        entries = [Entry(Path("a_test.go.bak"), "F", "debt", "x")]
        self.assertEqual(self.check([], entries, scope=[Path("a_test.go")]), [])


class TestRegistrySummary(unittest.TestCase):
    def test_summary(self):
        entries = [Entry(Path("a.go"), "F", "debt", "x"), Entry(Path("b.go"), "G", "untriaged", "y")]
        self.assertEqual(Registry(entries).summary(), "2 Internet access exceptions: 0 justified, 1 debt, 1 untriaged")
