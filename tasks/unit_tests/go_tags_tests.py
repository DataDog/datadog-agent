import json
import tempfile
import unittest
from pathlib import Path

from tasks.libs.build import go_tags


def _rule(name, kind, **attrs):
    attributes = []
    for attr, value in attrs.items():
        if isinstance(value, str):
            attributes.append({"name": attr, "stringValue": value})
        else:
            attributes.append({"name": attr, "stringListValue": value})
    return json.dumps({"type": "RULE", "rule": {"name": name, "ruleClass": kind, "attribute": attributes}})


class TestScanBuildTags(unittest.TestCase):
    def _scan(self, name, content):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / name
            path.write_text(content)
            return go_tags.scan_build_tags(str(path))

    def test_reads_both_constraint_syntaxes(self):
        content = "// Copyright\n//go:build linux && (docker || !kubelet)\n// +build linux\n\npackage foo\n"
        self.assertEqual(self._scan("a.go", content), {"linux", "docker", "kubelet"})

    def test_ignores_constraints_after_package_clause(self):
        self.assertEqual(self._scan("a.go", "package foo\n\n//go:build docker\n"), set())

    def test_reads_constraints_after_a_block_comment(self):
        content = "/*\nCopyright\n*/\n\n//go:build linux\n\n#include <stdio.h>\n//go:build docker\n"
        self.assertEqual(self._scan("a.c", content), {"linux"})

    def test_reads_dotted_tags(self):
        self.assertEqual(
            self._scan("a.go", "//go:build goexperiment.systemcrypto\npackage foo\n"), {"goexperiment.systemcrypto"}
        )

    def test_unreadable_file(self):
        self.assertIsNone(go_tags.scan_build_tags("/nonexistent/a.go"))


class TestScanSources(unittest.TestCase):
    def test_reuses_unchanged_files_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "a.go"
            path.write_text("//go:build linux\npackage foo\n")
            missing = str(Path(tmp) / "gen.go")
            tags, cache = go_tags.scan_sources([str(path), missing], {})
            self.assertEqual(tags, {str(path): {"linux"}, missing: None})
            self.assertEqual(list(cache), [str(path)])

            cache[str(path)][1] = ["cached"]
            self.assertEqual(go_tags.scan_sources([str(path)], cache)[0], {str(path): {"cached"}})

            path.write_text("//go:build docker\npackage foo\n")
            self.assertEqual(go_tags.scan_sources([str(path)], cache)[0], {str(path): {"docker"}})

    def test_other_files_only_need_to_exist(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "schema.json"
            path.write_text("//go:build docker\n")
            missing = str(Path(tmp) / "api.pb")
            self.assertEqual(go_tags.scan_sources([str(path), missing], {})[0], {str(path): set(), missing: None})


class TestBuildManifest(unittest.TestCase):
    def _manifest(self, rules, sources):
        nodes = go_tags.parse_graph("\n".join(rules))
        return go_tags.build_manifest(nodes, sources.get)

    def test_closure_tags_within_root_universe(self):
        universe, manifest = self._manifest(
            [
                _rule("//cmd:bin", "go_binary", deps=["//a:a", "@@ext//d:d"], gotags=["docker"]),
                _rule("//a:a", "go_library", importpath="ex/a", srcs=["//a:a.go"], deps=["@@ext//b:b"]),
                _rule("@@ext//b:b", "go_library", importpath="ex/b", srcs=["@@ext//b:b.go"]),
                _rule("//c:c", "go_library", importpath="ex/c", srcs=["//c:c.go"]),
                _rule("@@ext//d:d", "go_library", importpath="ex/d"),
            ],
            {"//a:a.go": set(), "@@ext//b:b.go": {"docker", "linux"}, "//c:c.go": {"linux"}},
        )
        self.assertEqual(universe, ["docker"])
        self.assertEqual(manifest, {"ex/a": ["docker"], "ex/b": ["docker"]})

    def test_external_roots_do_not_extend_universe(self):
        universe, _ = self._manifest(
            [_rule("@@ext//cmd:bin", "go_binary", gotags=["docker"]), _rule("//a:a", "go_library", importpath="ex/a")],
            {},
        )
        self.assertEqual(universe, [])

    def test_unreadable_source_keeps_all_tags_of_library_and_dependents(self):
        _, manifest = self._manifest(
            [
                _rule("//cmd:bin", "go_binary", deps=["//a:a"], gotags=["docker"]),
                _rule("//a:a", "go_library", importpath="ex/a", deps=["@@ext//gen:gen"]),
                _rule("@@ext//gen:gen", "go_library", importpath="ex/gen", srcs=["@@ext//gen:gen.go"]),
                _rule("//c:c", "go_library", importpath="ex/c"),
            ],
            {},
        )
        self.assertEqual(manifest, {"ex/a": None, "ex/gen": None})

    def test_rule_in_srcs_keeps_all_tags(self):
        tag_free = sorted(go_tags.TAG_FREE_GENERATORS)[0]
        _, manifest = self._manifest(
            [
                _rule("//cmd:bin", "go_binary", deps=["//a:a", "//b:b", "//c:c", "//d:d"], gotags=["docker"]),
                _rule("//a:a", "go_library", importpath="ex/a", srcs=["//a:gen_src"]),
                _rule("//b:b", "go_library", importpath="ex/b", srcs=["//b:b.go", "//b:schema.json"]),
                _rule("//c:c", "go_library", importpath="ex/c", srcs=[tag_free]),
                _rule("//d:d", "go_library", importpath="ex/d", srcs=["//d:api.pb"]),
            ],
            {"//b:b.go": set(), "//b:schema.json": set()},
        )
        self.assertEqual(manifest, {"ex/a": None, "ex/d": None})

    def test_tag_free_generators_use_schema_codegen(self):
        root = Path(__file__).parents[2]
        for label in go_tags.TAG_FREE_GENERATORS:
            package, _, name = label.removeprefix("@@//").partition(":")
            build = (root / package / "BUILD.bazel").read_text()
            rule = build[build.index(f'name = "{name}"') :].split("\n)", 1)[0]
            self.assertIn('tool = "//tasks/schema:codegen_settings_main"', rule, label)
        for name in ("codegen_init_settings.py", "codegen_settings_main.py", "merge_schema.py"):
            self.assertNotRegex((root / "tasks/schema" / name).read_text(), r"go:build|\+build", name)

    def test_graph_without_main_library_fails(self):
        with self.assertRaisesRegex(ValueError, "no main-repository go_library"):
            self._manifest([_rule("@@ext//b:b", "go_library", importpath="ex/b")], {})

    def test_missing_dep_fails(self):
        with self.assertRaisesRegex(ValueError, "//missing:x"):
            self._manifest([_rule("//a:a", "go_library", importpath="ex/a", deps=["//missing:x"])], {})

    def test_proto_compiler_deps_count(self):
        _, manifest = self._manifest(
            [
                _rule("//cmd:bin", "go_binary", deps=["//p:p_go"], gotags=["grpcnotrace"]),
                _rule("//p:p_go", "go_library", importpath="ex/p", embed=["//p:p_proto_go"]),
                _rule("//p:p_proto_go", "go_proto_library", importpath="ex/p", compilers=["//c:grpc"]),
                _rule("//c:grpc", "go_proto_compiler", deps=["@@ext//grpc:grpc"]),
                _rule("@@ext//grpc:grpc", "go_library", importpath="ex/grpc", srcs=["@@ext//grpc:trace.go"]),
            ],
            {"@@ext//grpc:trace.go": {"grpcnotrace"}},
        )
        self.assertEqual(manifest["ex/p"], ["grpcnotrace"])

    def test_render_sorts_import_paths(self):
        self.assertEqual(
            go_tags.render_manifest(["docker"], {"ex/c": None, "ex/b": [], "ex/a": ["docker"]}),
            "# Code generated by `dda inv go.write-tag-manifest` - DO NOT EDIT.\n"
            "# Resolve merge conflicts by rerunning it.\n"
            "\n"
            'TAG_UNIVERSE = ["docker"]\n'
            "\n"
            "CLOSURE_TAGS = {\n"
            '    "ex/a": ["docker"],\n'
            '    "ex/b": [],\n'
            '    "ex/c": None,\n'
            "}\n",
        )


class TestMainRepoErrors(unittest.TestCase):
    def test_only_errors_inside_the_workspace(self):
        stderr = (
            "ERROR: /ob/external/toolchains_msvc+/BUILD.bazel:3:10: no such package '@@winsdk//'\n"
            "ERROR: /ws/pkg/a/BUILD.bazel:3:8: //pkg/a:a_test: no such attribute 'x' in 'go_test' rule\n"
            "INFO: /ws/pkg/b/BUILD.bazel\n"
            "ERROR: /wsx/pkg/c/BUILD.bazel:1:1: not the workspace\n"
        )
        self.assertEqual(
            go_tags.main_repo_errors(stderr, "/ws", "/ob"),
            ["ERROR: /ws/pkg/a/BUILD.bazel:3:8: //pkg/a:a_test: no such attribute 'x' in 'go_test' rule"],
        )

    def test_output_base_inside_the_workspace(self):
        stderr = (
            "ERROR: /ws/.cache/bazel/ob/external/toolchains_msvc+/msbuild/BUILD.bazel:14:6: no such package"
            " '@@msbuild//': execvp(/ws/.cache/bazel/ob/external/msiutil/msiutil.exe, ...) failed\n"
            "ERROR: /ws/pkg/a/BUILD.bazel:3:8: no such package '@@x//': referenced by '//pkg/a:a'\n"
        )
        self.assertEqual(
            go_tags.main_repo_errors(stderr, "/ws", "/ws/.cache/bazel/ob"),
            ["ERROR: /ws/pkg/a/BUILD.bazel:3:8: no such package '@@x//': referenced by '//pkg/a:a'"],
        )

    def test_windows_workspace(self):
        stderr = "ERROR: C:/ws/pkg/a/BUILD.bazel:3:8: no such attribute\n"
        self.assertEqual(go_tags.main_repo_errors(stderr, "C:\\ws", "C:\\ob"), [stderr.strip()])


class TestSourcePath(unittest.TestCase):
    def test_main_and_external_repos(self):
        self.assertEqual(go_tags.source_path("@@//pkg/a:a.go", "/ws", "/ob"), "/ws/pkg/a/a.go")
        self.assertEqual(go_tags.source_path("//pkg/a:sub/a.go", "/ws", "/ob"), "/ws/pkg/a/sub/a.go")
        self.assertEqual(
            go_tags.source_path("@@gazelle++go_deps+foo//b:b.go", "/ws", "/ob"),
            "/ob/external/gazelle++go_deps+foo/b/b.go",
        )
