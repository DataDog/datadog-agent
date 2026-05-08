"""Pure-function tests for tasks.libs.otelcol_schema.

These cover the small primitives that don't require fixture schema
files or Go module cache state: ID generation, ref classification,
collision detection, namespace-relative parsing, and convert-time
bare-ref handling.

A future M5 may add fixture-driven tests for `build_bundle` end-to-
end. For now this file pins the building blocks so M4 changes can
move quickly without silent regressions.
"""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from tasks.libs.otelcol_schema._refs import (
    classify_ref,
    follow_relative,
    is_component_mode,
    parse_namespace_relative,
    repo_namespace_of,
    resolve_relative_go_path,
    schema_contains_type,
)
from tasks.libs.otelcol_schema.bundle import (
    IdMapping,
    SchemaSource,
    _canonicalise_ref,
    _flatten,
    _missing_id,
    _resolve_in_mapping,
    _Resolver,
    assign_ids,
    component_id,
    short_label,
)
from tasks.libs.otelcol_schema.convert import is_bare_ref
from tasks.libs.otelcol_schema.inventory import Component, RefStatus, resolve_relative


class TestShortLabel(unittest.TestCase):
    def test_strips_contrib_prefix(self):
        self.assertEqual(
            short_label("github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"),
            "pkg_datadog_config",
        )

    def test_strips_collector_core_prefix(self):
        self.assertEqual(
            short_label("go.opentelemetry.io/collector/config/confighttp"),
            "core_config_confighttp",
        )

    def test_strips_local_prefix(self):
        self.assertEqual(
            short_label("github.com/DataDog/datadog-agent/comp/otelcol/foo"),
            "local_comp_otelcol_foo",
        )

    def test_unknown_path_uses_full_encoded(self):
        self.assertEqual(short_label("example.com/foo/bar"), "example_com_foo_bar")


class TestComponentId(unittest.TestCase):
    def test_root(self):
        self.assertEqual(component_id("processor", "infraattributes"), "component__processor__infraattributes")

    def test_with_inner(self):
        self.assertEqual(
            component_id("exporter", "serializer", inner="metrics_config"),
            "component__exporter__serializer__metrics_config",
        )


class TestRepoNamespaceOf(unittest.TestCase):
    def test_contrib(self):
        self.assertEqual(
            repo_namespace_of("github.com/open-telemetry/opentelemetry-collector-contrib/pkg/foo"),
            "github.com/open-telemetry/opentelemetry-collector-contrib",
        )

    def test_core(self):
        self.assertEqual(
            repo_namespace_of("go.opentelemetry.io/collector/config/confighttp"),
            "go.opentelemetry.io/collector",
        )

    def test_unknown(self):
        self.assertIsNone(repo_namespace_of("example.com/foo"))

    def test_exact_match(self):
        self.assertEqual(
            repo_namespace_of("go.opentelemetry.io/collector"),
            "go.opentelemetry.io/collector",
        )


class TestClassifyRef(unittest.TestCase):
    def test_uri(self):
        self.assertEqual(classify_ref("https://example.com/schema").kind, "uri")

    def test_relative_dot(self):
        s = classify_ref("./internal/sub.config")
        self.assertEqual(s.kind, "relative")
        self.assertEqual(s.target_module, "./internal/sub")
        self.assertEqual(s.target_type, "config")

    def test_relative_parent_dot(self):
        # Verifies the bug fix: ../ refs are routed via the relative branch
        # so the resolver can use Path.resolve(), instead of being silently
        # mangled by an earlier lstrip("./") call.
        s = classify_ref("../sibling.config")
        self.assertEqual(s.kind, "relative")
        self.assertEqual(s.target_module, "../sibling")
        self.assertEqual(s.target_type, "config")

    def test_namespace_relative(self):
        self.assertEqual(classify_ref("/pkg/datadog/config.api_config").kind, "namespace_relative")

    def test_package_type(self):
        s = classify_ref("go.opentelemetry.io/collector/config/confighttp.client_config")
        self.assertEqual(s.kind, "package_type")
        self.assertEqual(s.target_module, "go.opentelemetry.io/collector/config/confighttp")
        self.assertEqual(s.target_type, "client_config")

    def test_bare(self):
        self.assertEqual(classify_ref("metrics_config").kind, "bare")


class TestParseNamespaceRelative(unittest.TestCase):
    def test_typical(self):
        self.assertEqual(
            parse_namespace_relative(
                "/pkg/datadog/config.api_config",
                "github.com/open-telemetry/opentelemetry-collector-contrib",
            ),
            (
                "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config",
                "api_config",
            ),
        )

    def test_returns_none_for_non_namespace_relative(self):
        self.assertIsNone(parse_namespace_relative("foo.bar", "ns"))
        self.assertIsNone(parse_namespace_relative("/", "ns"))

    def test_returns_none_when_no_dot_separator(self):
        self.assertIsNone(parse_namespace_relative("/foo/bar", "ns"))


class TestSchemaContainsType(unittest.TestCase):
    def test_defs_entry(self):
        self.assertTrue(schema_contains_type({"$defs": {"foo": {}}}, "foo"))

    def test_root_config_in_component_mode(self):
        self.assertTrue(schema_contains_type({"properties": {"x": {}}}, "config"))
        self.assertTrue(schema_contains_type({"allOf": [{}]}, "config"))

    def test_root_non_config_not_implicitly(self):
        self.assertFalse(schema_contains_type({"properties": {"x": {}}}, "other"))

    def test_no_defs_no_root(self):
        self.assertFalse(schema_contains_type({}, "anything"))


class TestIsComponentMode(unittest.TestCase):
    def test_component_mode_when_root_type(self):
        self.assertTrue(is_component_mode({"type": "object"}))

    def test_component_mode_when_root_properties(self):
        self.assertTrue(is_component_mode({"properties": {}}))

    def test_component_mode_when_root_allof(self):
        self.assertTrue(is_component_mode({"allOf": []}))

    def test_package_mode_when_only_defs(self):
        self.assertFalse(is_component_mode({"$defs": {"x": {}}}))


class TestIsBareRef(unittest.TestCase):
    def test_bare(self):
        self.assertTrue(is_bare_ref("foo"))

    def test_not_bare_pointer(self):
        self.assertFalse(is_bare_ref("#/$defs/foo"))

    def test_not_bare_uri(self):
        self.assertFalse(is_bare_ref("https://example.com"))

    def test_not_bare_with_slash(self):
        self.assertFalse(is_bare_ref("foo/bar"))

    def test_not_bare_with_dot(self):
        self.assertFalse(is_bare_ref("foo.bar"))

    def test_empty(self):
        self.assertFalse(is_bare_ref(""))


class TestFlatten(unittest.TestCase):
    def test_replaces_separators(self):
        self.assertEqual(_flatten("foo/bar.baz-qux"), "foo_bar_baz_qux")

    def test_leaves_safe_chars(self):
        self.assertEqual(_flatten("snake_case"), "snake_case")


class TestMissingId(unittest.TestCase):
    """`_missing_id` must produce stable, collision-free IDs for unresolved
    refs even when their `_flatten` stems collide."""

    def test_format_includes_prefix_stem_and_hash(self):
        out = _missing_id("foo.bar")
        self.assertTrue(out.startswith("__missing__foo_bar__"))
        # 8-char hex suffix
        self.assertRegex(out, r"^__missing__foo_bar__[0-9a-f]{8}$")

    def test_distinct_refs_with_same_flatten_get_distinct_ids(self):
        # `_flatten` collapses `/` and `.` to `_`, so these two refs would
        # collide without the hash suffix.
        a = _missing_id("foo.bar")
        b = _missing_id("foo/bar")
        self.assertNotEqual(a, b)

    def test_same_input_is_stable(self):
        self.assertEqual(_missing_id("foo.bar"), _missing_id("foo.bar"))


class TestResolveInMapping(unittest.TestCase):
    def test_defs_lookup(self):
        m = IdMapping(root_id=None, defs_ids={"a": "shared__pkg__a"})
        self.assertEqual(_resolve_in_mapping(m, "a"), "shared__pkg__a")

    def test_root_fallback_for_config(self):
        m = IdMapping(root_id="component__processor__foo", defs_ids={})
        self.assertEqual(_resolve_in_mapping(m, "config"), "component__processor__foo")

    def test_root_fallback_only_for_config_name(self):
        m = IdMapping(root_id="component__processor__foo", defs_ids={})
        self.assertIsNone(_resolve_in_mapping(m, "other_name"))

    def test_unknown(self):
        m = IdMapping(root_id=None, defs_ids={})
        self.assertIsNone(_resolve_in_mapping(m, "anything"))


class TestAssignIdsCollision(unittest.TestCase):
    """Confirm `assign_ids` raises when two sources claim the same canonical
    ID. With the live naming scheme this can't happen, so we deliberately
    construct a collision by handing two sources the same component
    (class, type)."""

    def test_collision_raises(self):
        s1 = SchemaSource(
            path=Path("/tmp/a/config.schema.yaml"),
            go_path="github.com/x/foo",
            doc={"properties": {}},
            component_class="processor",
            component_type="dup",
        )
        s2 = SchemaSource(
            path=Path("/tmp/b/config.schema.yaml"),
            go_path="github.com/x/bar",
            doc={"properties": {}},
            component_class="processor",
            component_type="dup",
        )
        with self.assertRaises(RuntimeError) as cm:
            assign_ids([s1, s2])
        self.assertIn("collision", str(cm.exception))
        self.assertIn("component__processor__dup", str(cm.exception))

    def test_no_collision_for_distinct_components(self):
        s1 = SchemaSource(
            path=Path("/tmp/a/config.schema.yaml"),
            go_path="github.com/x/a",
            doc={"properties": {}},
            component_class="processor",
            component_type="a",
        )
        s2 = SchemaSource(
            path=Path("/tmp/b/config.schema.yaml"),
            go_path="github.com/x/b",
            doc={"properties": {}, "$defs": {"inner": {}}},
            component_class="processor",
            component_type="b",
        )
        mappings = assign_ids([s1, s2])
        self.assertEqual(mappings[s1.path].root_id, "component__processor__a")
        self.assertEqual(mappings[s2.path].root_id, "component__processor__b")
        self.assertEqual(mappings[s2.path].defs_ids, {"inner": "component__processor__b__inner"})


class TestResolveRelativeGoPath(unittest.TestCase):
    """Pin Go-import-path semantics for relative refs. Regression test for
    a bug where `lstrip("./").replace("..", "")` mishandled `../` refs by
    silently turning them into same-dir lookups."""

    def test_dot_slash_appends_segments(self):
        self.assertEqual(
            resolve_relative_go_path("github.com/foo/bar", "./internal/sub"),
            "github.com/foo/bar/internal/sub",
        )

    def test_parent_pops_one_segment(self):
        self.assertEqual(
            resolve_relative_go_path("github.com/foo/bar", "../sibling"),
            "github.com/foo/sibling",
        )

    def test_double_parent_pops_two(self):
        self.assertEqual(
            resolve_relative_go_path("a/b/c", "../../sibling"),
            "a/sibling",
        )

    def test_dot_only_is_idempotent(self):
        self.assertEqual(resolve_relative_go_path("a/b", "./."), "a/b")

    def test_empty_source_starts_fresh(self):
        self.assertEqual(resolve_relative_go_path("", "./foo"), "foo")

    def test_pop_past_root_is_clamped(self):
        # Defensive: walking ".." past the source root should not error.
        self.assertEqual(resolve_relative_go_path("a", "../../foo"), "foo")


class TestFollowRelative(unittest.TestCase):
    """Filesystem-bound test for `follow_relative` using tmp dirs."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        # Layout:
        #   root/comp/impl/         (source)
        #   root/comp/impl/internal/sub/config.schema.yaml
        #   root/comp/sibling/config.schema.yaml
        (self.root / "comp" / "impl" / "internal" / "sub").mkdir(parents=True)
        (self.root / "comp" / "sibling").mkdir(parents=True)
        (self.root / "comp" / "impl" / "internal" / "sub" / "config.schema.yaml").write_text("type: object\n")
        (self.root / "comp" / "sibling" / "config.schema.yaml").write_text("type: object\n")
        self.source_dir = self.root / "comp" / "impl"

    def tearDown(self):
        self.tmp.cleanup()

    def test_dot_slash_finds_sibling_subdir(self):
        result = follow_relative(self.source_dir, "./internal/sub")
        self.assertIsNotNone(result)
        assert result is not None  # for type-checkers
        self.assertEqual(result.name, "config.schema.yaml")
        self.assertEqual(result.parent.name, "sub")

    def test_parent_resolves_via_path_resolve(self):
        result = follow_relative(self.source_dir, "../sibling")
        self.assertIsNotNone(result)
        assert result is not None
        self.assertEqual(result.parent.name, "sibling")

    def test_missing_target_returns_none(self):
        self.assertIsNone(follow_relative(self.source_dir, "./nope"))

    def test_empty_target_returns_none(self):
        self.assertIsNone(follow_relative(self.source_dir, None))
        self.assertIsNone(follow_relative(self.source_dir, ""))


def _make_resolver(
    sources_by_path: dict[Path, IdMapping] | None = None,
    *,
    versions: dict[str, str] | None = None,
    cache_root: Path | None = None,
) -> _Resolver:
    return _Resolver(
        sources_by_path=sources_by_path or {},
        versions=versions or {},
        cache_root=cache_root or Path("/nonexistent"),
        unresolved={},
    )


def _make_source(path: str = "/tmp/test/config.schema.yaml", **kwargs) -> SchemaSource:
    """Build a SchemaSource for ref-rewriting tests. Defaults to a local
    Datadog component path so namespace_relative resolution works."""
    defaults = {
        "go_path": "github.com/DataDog/datadog-agent/comp/x/impl",
        "doc": {},
        "component_class": "extension",
        "component_type": "test",
    }
    defaults.update(kwargs)
    return SchemaSource(path=Path(path), **defaults)  # type: ignore[arg-type]


class TestCanonicaliseRef(unittest.TestCase):
    """The dispatch table inside `_canonicalise_ref` is the most exposed
    surface for M4/M5 refactors. Every ref kind gets at least one hit and
    one miss to anchor the behaviour."""

    def test_already_rewritten_pointer_translates_via_own_defs(self):
        source = _make_source()
        own_defs_ids = {"foo": "component__extension__test__foo"}
        out = _canonicalise_ref("#/$defs/foo", source=source, own_defs_ids=own_defs_ids, resolver=_make_resolver())
        self.assertEqual(out, "#/$defs/component__extension__test__foo")

    def test_already_rewritten_pointer_miss_records_unresolved(self):
        resolver = _make_resolver()
        source = _make_source()
        out = _canonicalise_ref("#/$defs/missing", source=source, own_defs_ids={}, resolver=resolver)
        self.assertTrue(out.startswith("#/$defs/__missing__"))
        self.assertIn("#/$defs/missing", resolver.unresolved)

    def test_other_fragment_pointer_passes_through(self):
        source = _make_source()
        out = _canonicalise_ref("#/properties/foo", source=source, own_defs_ids={}, resolver=_make_resolver())
        self.assertEqual(out, "#/properties/foo")

    def test_bare_hit_translates(self):
        source = _make_source()
        own_defs_ids = {"my_type": "component__extension__test__my_type"}
        out = _canonicalise_ref("my_type", source=source, own_defs_ids=own_defs_ids, resolver=_make_resolver())
        self.assertEqual(out, "#/$defs/component__extension__test__my_type")

    def test_bare_miss_records_unresolved(self):
        resolver = _make_resolver()
        source = _make_source()
        out = _canonicalise_ref("ghost", source=source, own_defs_ids={}, resolver=resolver)
        self.assertTrue(out.startswith("#/$defs/__missing__"))
        self.assertIn("ghost", resolver.unresolved)

    def test_package_type_miss_records_unresolved(self):
        # No version known + empty sources_by_path => placeholder.
        resolver = _make_resolver()
        source = _make_source()
        out = _canonicalise_ref(
            "go.opentelemetry.io/collector/pipeline.id",
            source=source,
            own_defs_ids={},
            resolver=resolver,
        )
        self.assertTrue(out.startswith("#/$defs/__missing__"))
        self.assertIn("go.opentelemetry.io/collector/pipeline.id", resolver.unresolved)

    def test_relative_miss_records_unresolved(self):
        # `follow_relative` returns None because the target file doesn't exist.
        resolver = _make_resolver()
        source = _make_source()
        out = _canonicalise_ref("./internal/nope.foo", source=source, own_defs_ids={}, resolver=resolver)
        self.assertTrue(out.startswith("#/$defs/__missing__"))
        self.assertIn("./internal/nope.foo", resolver.unresolved)

    def test_namespace_relative_miss_records_unresolved(self):
        # source go_path is in DataDog repo namespace, but no module schema is
        # registered so the lookup falls through to the placeholder.
        resolver = _make_resolver()
        source = _make_source()
        out = _canonicalise_ref("/comp/x/foo.bar", source=source, own_defs_ids={}, resolver=resolver)
        self.assertTrue(out.startswith("#/$defs/__missing__"))
        self.assertIn("/comp/x/foo.bar", resolver.unresolved)

    def test_namespace_relative_with_unknown_namespace_records_unresolved(self):
        resolver = _make_resolver()
        source = _make_source(go_path="example.com/random")  # not a known namespace
        out = _canonicalise_ref("/foo/bar.baz", source=source, own_defs_ids={}, resolver=resolver)
        self.assertTrue(out.startswith("#/$defs/__missing__"))

    def test_uri_passes_through(self):
        resolver = _make_resolver()
        source = _make_source()
        out = _canonicalise_ref(
            "https://example.com/schema.json",
            source=source,
            own_defs_ids={},
            resolver=resolver,
        )
        self.assertEqual(out, "https://example.com/schema.json")
        self.assertEqual(resolver.unresolved, {})

    def test_relative_hit_resolves_via_filesystem_and_mapping(self):
        # End-to-end test using a tmp dir + populated `sources_by_path`.
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "src").mkdir()
            (root / "src" / "internal" / "sub").mkdir(parents=True)
            target_file = root / "src" / "internal" / "sub" / "config.schema.yaml"
            target_file.write_text("type: object\n")
            target_file = target_file.resolve()
            mapping = IdMapping(root_id="shared__pkg_sub", defs_ids={"foo": "shared__pkg_sub__foo"})
            resolver = _make_resolver({target_file: mapping})
            source = _make_source(path=str(root / "src" / "config.schema.yaml"))
            out = _canonicalise_ref("./internal/sub.foo", source=source, own_defs_ids={}, resolver=resolver)
            self.assertEqual(out, "#/$defs/shared__pkg_sub__foo")

    def test_relative_hit_falls_back_to_root_for_config_name(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "src" / "internal" / "sub").mkdir(parents=True)
            target_file = (root / "src" / "internal" / "sub" / "config.schema.yaml").resolve()
            target_file.write_text("type: object\n")
            mapping = IdMapping(root_id="component__processor__sub", defs_ids={})
            resolver = _make_resolver({target_file: mapping})
            source = _make_source(path=str(root / "src" / "config.schema.yaml"))
            out = _canonicalise_ref("./internal/sub.config", source=source, own_defs_ids={}, resolver=resolver)
            self.assertEqual(out, "#/$defs/component__processor__sub")


def _make_component(schema_path: Path | str | None) -> Component:
    """Minimal `Component` for resolve_relative tests. Only schema_path is
    consulted by `resolve_relative`; everything else is filler."""
    return Component(
        source="local",
        section="processors",
        gomod=None,
        version=None,
        module_dir=None,
        in_cache=True,
        metadata_path=None,
        metadata_type=None,
        metadata_class=None,
        schema_path=str(schema_path) if schema_path is not None else None,
        schema_present=schema_path is not None,
    )


class TestInventoryResolveRelative(unittest.TestCase):
    """Pin diagnostic granularity and the multi-consumer fall-through that
    parallels bundle's relative-ref handling. Mirror of `TestFollowRelative`
    plus the type-found / type-not-found / dir-missing cases the previous
    implementation collapsed."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        # Layout:
        #   root/src1/config.schema.yaml         (consumer 1)
        #   root/src2/config.schema.yaml         (consumer 2)
        #   root/src1/internal/has_type/config.schema.yaml  ($defs has 'foo')
        #   root/src1/internal/wrong_type/config.schema.yaml ($defs has 'bar')
        #   root/src1/internal/dir_only/         (no schema file)
        for sub in ("src1", "src2", "src1/internal/has_type", "src1/internal/wrong_type", "src1/internal/dir_only"):
            (self.root / sub).mkdir(parents=True, exist_ok=True)
        for sub in ("src1", "src2"):
            (self.root / sub / "config.schema.yaml").write_text("type: object\n")
        (self.root / "src1" / "internal" / "has_type" / "config.schema.yaml").write_text(
            "$defs:\n  foo: {type: object}\n"
        )
        (self.root / "src1" / "internal" / "wrong_type" / "config.schema.yaml").write_text(
            "$defs:\n  bar: {type: object}\n"
        )

    def tearDown(self):
        self.tmp.cleanup()

    def _consumer(self, sub: str) -> Component:
        return _make_component(self.root / sub / "config.schema.yaml")

    def _status(self, ref: str) -> RefStatus:
        s = classify_ref(ref)
        self.assertEqual(s.kind, "relative")
        return s

    def test_resolved_when_type_present(self):
        status = self._status("./internal/has_type.foo")
        resolve_relative(status, consumers=[self._consumer("src1")])
        self.assertTrue(status.resolved)
        self.assertIn("has_type", status.schema_path or "")

    def test_falls_through_to_second_consumer(self):
        # Regression test: `./internal/has_type.foo` only resolves when we
        # iterate past the consumer whose dir doesn't contain it.
        status = self._status("./internal/has_type.foo")
        # src2 has no `internal/has_type`; src1 does. Old buggy code stopped
        # at the first consumer's "type not found" branch.
        resolve_relative(status, consumers=[self._consumer("src2"), self._consumer("src1")])
        self.assertTrue(status.resolved)

    def test_type_missing_in_present_file(self):
        status = self._status("./internal/wrong_type.missing_t")
        resolve_relative(status, consumers=[self._consumer("src1")])
        self.assertFalse(status.resolved)
        self.assertIn("'missing_t' not found", status.note)
        self.assertIn("wrong_type", status.schema_path or "")

    def test_sibling_dir_exists_but_no_schema(self):
        status = self._status("./internal/dir_only.foo")
        resolve_relative(status, consumers=[self._consumer("src1")])
        self.assertFalse(status.resolved)
        self.assertIn("cached but no config.schema.yaml", status.note)

    def test_sibling_dir_does_not_exist(self):
        status = self._status("./internal/nope.foo")
        resolve_relative(status, consumers=[self._consumer("src1")])
        self.assertFalse(status.resolved)
        self.assertIn("does not exist", status.note)

    def test_parent_relative_resolves(self):
        # Regression test for the `../` bug fix. Layout:
        #   root/src1/internal/has_type/config.schema.yaml (the target)
        #   root/src1/internal/dir_only/                   (the source dir)
        # A `../has_type.foo` ref from `dir_only` should hit `has_type`.
        consumer = _make_component(self.root / "src1" / "internal" / "dir_only" / "consumer.schema.yaml")
        status = self._status("../has_type.foo")
        resolve_relative(status, consumers=[consumer])
        self.assertTrue(status.resolved)
        self.assertIn("has_type", status.schema_path or "")

    def test_malformed_candidate_does_not_block_other_consumers(self):
        # Regression test: a YAML parse error under one consumer's sibling
        # dir must not short-circuit the multi-consumer fall-through. Set
        # up `src2/internal/has_type/config.schema.yaml` with corrupt YAML
        # so it's tried first; src1's clean copy must still resolve.
        bad_dir = self.root / "src2" / "internal" / "has_type"
        bad_dir.mkdir(parents=True)
        (bad_dir / "config.schema.yaml").write_text(
            "type: object\n" "  this:: is not valid yaml\n" "  - mixed: [up, with, lists\n"
        )
        status = self._status("./internal/has_type.foo")
        resolve_relative(
            status,
            consumers=[self._consumer("src2"), self._consumer("src1")],
        )
        self.assertTrue(status.resolved)
        self.assertIn("has_type", status.schema_path or "")

    def test_only_malformed_candidate_reports_parse_error(self):
        # When every candidate parses fail, `parse error` is the diagnostic.
        bad_dir = self.root / "src1" / "internal" / "broken"
        bad_dir.mkdir(parents=True)
        (bad_dir / "config.schema.yaml").write_text("foo: [\n")
        status = self._status("./internal/broken.foo")
        resolve_relative(status, consumers=[self._consumer("src1")])
        self.assertFalse(status.resolved)
        self.assertIn("parse error", status.note)
        self.assertIn("broken", status.schema_path or "")

    def test_parse_error_priority_beats_type_not_found(self):
        # Pin the diagnostic priority order: a parse error in one consumer's
        # candidate must take precedence over a type-not-found result from a
        # later consumer's well-formed-but-wrong-type candidate. Parse errors
        # are actionable (fix the YAML); pointing the operator at the
        # wrong-type file would hide the real problem.
        bad_dir = self.root / "src2" / "internal" / "wrong_type"
        bad_dir.mkdir(parents=True)
        (bad_dir / "config.schema.yaml").write_text("foo: [\n")
        # `wrong_type.bar` exists in src1 (defined in setUp), parse-fails in src2.
        status = self._status("./internal/wrong_type.unknown_t")
        resolve_relative(
            status,
            consumers=[self._consumer("src2"), self._consumer("src1")],
        )
        self.assertFalse(status.resolved)
        self.assertIn("parse error", status.note)
        self.assertIn("src2", status.schema_path or "")


if __name__ == "__main__":
    unittest.main()
