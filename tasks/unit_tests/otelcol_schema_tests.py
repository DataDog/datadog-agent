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
    _stitch_envelope,
    assign_ids,
    build_bundle,
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


def _component_source(cls: str, type_name: str, *, path: str = "/tmp/c.yaml") -> SchemaSource:
    """Build a `SchemaSource` representing a registered component for stitch tests."""
    return SchemaSource(
        path=Path(path),
        go_path=f"github.com/example/{type_name}",
        doc={"type": "object"},  # component-mode
        component_class=cls,
        component_type=type_name,
    )


class TestStitchEnvelope(unittest.TestCase):
    """Pin envelope-stitching behaviour: registered components produce
    `patternProperties` entries; missing-component classes get a fallback
    only under permissive strategy."""

    def test_registered_component_produces_pattern_entry(self):
        src = _component_source("receiver", "otlp")
        mappings = {src.path: IdMapping(root_id="component__receiver__otlp", defs_ids={})}
        env = _stitch_envelope([src], mappings, [], missing_strategy="strict")
        receivers_pp = env["properties"]["receivers"]["patternProperties"]
        self.assertIn(r"^otlp(/.*)?$", receivers_pp)
        self.assertEqual(receivers_pp[r"^otlp(/.*)?$"]["$ref"], "#/$defs/component__receiver__otlp")

    def test_class_mapping_for_all_five_classes(self):
        # Build one component per class to confirm class -> envelope key mapping.
        sources = []
        mappings = {}
        for cls in ("receiver", "processor", "exporter", "connector", "extension"):
            src = _component_source(cls, f"my_{cls}", path=f"/tmp/{cls}.yaml")
            sources.append(src)
            mappings[src.path] = IdMapping(root_id=f"component__{cls}__my_{cls}", defs_ids={})
        env = _stitch_envelope(sources, mappings, [], missing_strategy="strict")
        for cls, key in [
            ("receiver", "receivers"),
            ("processor", "processors"),
            ("exporter", "exporters"),
            ("connector", "connectors"),
            ("extension", "extensions"),
        ]:
            pp = env["properties"][key]["patternProperties"]
            self.assertIn(rf"^my_{cls}(/.*)?$", pp)

    def test_strict_mode_no_fallback_for_missing_classes(self):
        env = _stitch_envelope([], {}, [("receiver", "go.example/foo", "no schema")], missing_strategy="strict")
        self.assertNotIn("^.*$", env["properties"]["receivers"]["patternProperties"])

    def test_permissive_mode_adds_fallback_only_for_classes_with_gaps(self):
        # `processor` has a gap, `exporter` doesn't. Only processors get the
        # fallback pattern.
        missing = [("processor", "go.example/p", "no schema")]
        env = _stitch_envelope([], {}, missing, missing_strategy="permissive")
        self.assertIn("^.*$", env["properties"]["processors"]["patternProperties"])
        self.assertNotIn("^.*$", env["properties"]["exporters"]["patternProperties"])
        self.assertEqual(
            env["properties"]["processors"]["patternProperties"]["^.*$"]["$ref"],
            "#/$defs/__component__permissive__",
        )

    def test_unknown_class_in_missing_list_is_ignored(self):
        # `widget` isn't a Collector class; envelope shouldn't grow.
        env = _stitch_envelope([], {}, [("widget", "go.example/foo", "no schema")], missing_strategy="permissive")
        for key in ("receivers", "processors", "exporters", "connectors", "extensions"):
            self.assertNotIn("^.*$", env["properties"][key]["patternProperties"])

    def test_multi_instance_pipeline_pattern_matches(self):
        # The `(/.*)?` suffix on signal patterns is the whole point of letting
        # operators define multiple pipelines per signal (`traces/foo`,
        # `traces/bar`). Pin that the regex actually matches the upstream
        # convention: `<signal>` and `<signal>/<instance>`.
        import re as _re

        env = _stitch_envelope([], {}, [], missing_strategy="strict")
        signal_pat = next(iter(env["properties"]["service"]["properties"]["pipelines"]["patternProperties"]))
        for ok in ("traces", "metrics/foo", "logs/api-gateway", "profiles/cpu_only"):
            self.assertTrue(_re.match(signal_pat, ok), f"{ok!r} should match {signal_pat!r}")
        # The pattern is permissive about the instance suffix (`traces/`
        # matches with an empty instance) — upstream validates component-id
        # syntax at runtime, the schema only enforces shape. So we only
        # assert that obviously-wrong signals are rejected.
        for nope in ("bogus", "tracesx", "/traces"):
            m = _re.match(signal_pat, nope)
            self.assertFalse(
                m and m.group(0) == nope,
                f"{nope!r} should not fully match {signal_pat!r}",
            )


class TestBuildBundleEndToEnd(unittest.TestCase):
    """One smoke test that drives the full `build_bundle` pipeline and
    validates a minimal Collector config against the resulting schema.

    This is the only test that exercises the discovery → assign_ids →
    rewrite → stitch → assemble chain end-to-end. It catches envelope-shape
    regressions that the unit tests around `_stitch_envelope` can't see —
    e.g. broken `service.pipelines` shape or missing top-level keys.

    Skips gracefully if jsonschema is not available; M5 will lift it into
    project-wide requirements.
    """

    def test_minimal_valid_config_passes(self):
        try:
            from jsonschema import Draft202012Validator
        except ImportError:
            self.skipTest("jsonschema not available in this environment")

        result = build_bundle(missing_strategy="permissive")
        Draft202012Validator.check_schema(result.bundle)

        # A minimal Collector config that exercises every envelope branch:
        # at least one receiver, processor, exporter, extension, and a
        # multi-instance pipeline name.
        config = {
            "receivers": {"otlp": {}},
            "processors": {"batch": {}},
            "exporters": {"logsagent": {}},
            "extensions": {"ddflare": {}},
            "service": {
                "extensions": ["ddflare"],
                "pipelines": {
                    "logs": {"receivers": ["otlp"], "processors": ["batch"], "exporters": ["logsagent"]},
                    "logs/secondary": {"receivers": ["otlp"], "exporters": ["logsagent"]},
                },
            },
        }
        errors = list(Draft202012Validator(result.bundle).iter_errors(config))
        self.assertEqual(errors, [], f"unexpected errors: {[e.message for e in errors[:3]]}")

    def test_top_level_typo_is_caught(self):
        try:
            from jsonschema import Draft202012Validator
        except ImportError:
            self.skipTest("jsonschema not available in this environment")

        result = build_bundle(missing_strategy="permissive")
        config = {"recievers": {}, "service": {"pipelines": {}}}
        errors = list(Draft202012Validator(result.bundle).iter_errors(config))
        self.assertTrue(
            any("recievers" in e.message for e in errors),
            f"expected 'recievers' typo error, got: {[e.message for e in errors[:3]]}",
        )


class TestOtelcolSchemaInvokeTasks(unittest.TestCase):
    """Exercises the `tasks.otelcol_schema` invoke task surface.

    Drives the tasks in-process with a real `invoke.Context` rather than
    shelling out to `dda inv` so the test suite stays fast and doesn't
    depend on the dda venv state.
    """

    def setUp(self):
        from invoke import Context

        self.ctx = Context()
        self.tmp = tempfile.TemporaryDirectory()
        self.tmp_path = Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def test_gen_writes_bundle(self):
        from tasks.otelcol_schema import gen

        out = self.tmp_path / "bundle.json"
        gen(self.ctx, output=str(out), no_download=True)
        self.assertTrue(out.is_file())
        # Bundle is valid JSON and structurally a JSON Schema 2020-12 doc.
        import json

        bundle = json.loads(out.read_text())
        self.assertEqual(bundle["$schema"], "https://json-schema.org/draft/2020-12/schema")
        self.assertIn("$defs", bundle)
        self.assertIn("properties", bundle)

    def test_check_passes_for_matching_bundle(self):
        from tasks.otelcol_schema import check, gen

        out = self.tmp_path / "bundle.json"
        gen(self.ctx, output=str(out), no_download=True)
        # Should not raise: the just-generated bundle matches itself.
        check(self.ctx, against=str(out), no_download=True)

    def test_check_fails_for_drifted_bundle(self):
        from invoke.exceptions import Exit

        from tasks.otelcol_schema import check, gen

        out = self.tmp_path / "bundle.json"
        gen(self.ctx, output=str(out), no_download=True)

        # Inject a drift and confirm `check` reports it.
        import json

        bundle = json.loads(out.read_text())
        bundle["$defs"]["__corrupted_for_test__"] = {"type": "string"}
        out.write_text(json.dumps(bundle, indent=2) + "\n")

        with self.assertRaises(Exit) as cm:
            check(self.ctx, against=str(out), no_download=True)
        self.assertEqual(cm.exception.code, 1)
        self.assertIn("out of date", str(cm.exception.message))

    def test_check_errors_when_artifact_missing(self):
        from invoke.exceptions import Exit

        from tasks.otelcol_schema import check

        missing = self.tmp_path / "does-not-exist.json"
        with self.assertRaises(Exit) as cm:
            check(self.ctx, against=str(missing), no_download=True)
        self.assertEqual(cm.exception.code, 2)

    def test_gen_with_validate_errors_when_jsonschema_missing(self):
        """When --validate is requested and jsonschema isn't importable,
        the task must fail with a clear exit code rather than silently
        skipping. Simulated by monkey-patching `jsonschema_available`."""
        from invoke.exceptions import Exit

        from tasks.libs.otelcol_schema import convert as convert_mod
        from tasks.otelcol_schema import gen

        original = convert_mod.jsonschema_available
        convert_mod.jsonschema_available = lambda: False
        try:
            out = self.tmp_path / "bundle.json"
            with self.assertRaises(Exit) as cm:
                gen(self.ctx, output=str(out), validate=True, no_download=True)
            self.assertEqual(cm.exception.code, 2)
            self.assertIn("jsonschema", str(cm.exception.message))
        finally:
            convert_mod.jsonschema_available = original

    def test_gen_with_validate_passes_when_jsonschema_available(self):
        """Happy path for --validate: jsonschema is installed, so validation
        runs and succeeds (the bundler's output is JSON-Schema-2020-12 valid
        by construction)."""
        from tasks.libs.otelcol_schema.convert import jsonschema_available
        from tasks.otelcol_schema import gen

        if not jsonschema_available():
            self.skipTest("jsonschema not available in this environment")
        out = self.tmp_path / "bundle.json"
        gen(self.ctx, output=str(out), validate=True, no_download=True)
        self.assertTrue(out.is_file())

    def test_gen_writes_report_when_requested(self):
        """--report writes a markdown summary alongside the bundle."""
        from tasks.otelcol_schema import gen

        out = self.tmp_path / "bundle.json"
        report = self.tmp_path / "report.md"
        gen(self.ctx, output=str(out), report=str(report), no_download=True)
        self.assertTrue(report.is_file())
        content = report.read_text()
        # Sanity: the report should mention component classes the bundler covered.
        self.assertIn("Components included", content)

    def test_gen_translates_runtime_error_to_exit_2(self):
        """Pins the RuntimeError → Exit(2) translation in `gen` without
        coupling to the live manifest's unresolved-ref count. Monkey-patch
        `build_bundle` to raise; the task wrapper must surface the message
        on Exit and preserve the underlying error."""
        from invoke.exceptions import Exit

        from tasks.libs.otelcol_schema import bundle as bundle_mod
        from tasks.otelcol_schema import gen

        original = bundle_mod.build_bundle

        def _raising(*, missing_strategy):  # noqa: ARG001 — match the real signature
            raise RuntimeError("strict mode: unresolved refs: sample.ref")

        bundle_mod.build_bundle = _raising
        try:
            out = self.tmp_path / "bundle.json"
            with self.assertRaises(Exit) as cm:
                gen(self.ctx, output=str(out), missing="strict", no_download=True)
            self.assertEqual(cm.exception.code, 2)
            self.assertIn("strict mode", str(cm.exception.message))
            self.assertIn("sample.ref", str(cm.exception.message))
        finally:
            bundle_mod.build_bundle = original

    def test_gen_pre_flight_raises_exit_when_download_fails(self):
        """When `gen` is called without `--no-download` and the pre-flight
        download of a manifest module fails, the task must Exit(2) with a
        message that includes both the failing spec and the `--no-download`
        remediation hint. Simulated by monkey-patching the lib helper to
        return a synthetic failure."""
        from invoke.exceptions import Exit

        from tasks.libs.otelcol_schema import bundle as bundle_mod
        from tasks.otelcol_schema import gen

        original = bundle_mod.ensure_manifest_modules_downloaded
        bundle_mod.ensure_manifest_modules_downloaded = lambda: [
            ("github.com/example/foo@v1.2.3", "network unreachable")
        ]
        try:
            out = self.tmp_path / "bundle.json"
            with self.assertRaises(Exit) as cm:
                gen(self.ctx, output=str(out))  # note: no_download defaults to False
            self.assertEqual(cm.exception.code, 2)
            message = str(cm.exception.message)
            self.assertIn("could not download", message)
            self.assertIn("github.com/example/foo@v1.2.3", message)
            self.assertIn("network unreachable", message)
            self.assertIn("--no-download", message)
        finally:
            bundle_mod.ensure_manifest_modules_downloaded = original

    def test_inventory_writes_reports(self):
        """The `inventory` task wraps the M1 script and writes both JSON
        and markdown reports to caller-supplied paths."""
        from tasks.otelcol_schema import inventory

        json_out = self.tmp_path / "inventory.json"
        md_out = self.tmp_path / "inventory.md"
        inventory(self.ctx, json_out=str(json_out), md=str(md_out), no_download=True)
        self.assertTrue(json_out.is_file())
        self.assertTrue(md_out.is_file())
        # Sanity: the JSON parses and reports a non-trivial component count.
        import json

        data = json.loads(json_out.read_text())
        self.assertGreater(data["summary"]["total_components"], 0)


class TestUpstreamShims(unittest.TestCase):
    """Pin the upstream-shim mechanism: locally-shipped schemas override the
    cached upstream module's lack of one. The shim is consulted by both
    the manifest-component walk and the transitive `$ref` resolver.
    """

    def test_registered_shims_have_schema_files_on_disk(self):
        """The UPSTREAM_SHIMS table is the source of truth — every entry
        must point at a directory that actually contains a config.schema.yaml,
        otherwise the bundler silently falls back to the cache and the
        intent of the shim is lost."""
        from pathlib import Path

        from tasks.libs.otelcol_schema._refs import find_module_schema  # noqa: F401 — ensure module loads
        from tasks.libs.otelcol_schema.bundle import REPO_ROOT, UPSTREAM_SHIMS

        for go_path, rel in UPSTREAM_SHIMS.items():
            schema = Path(REPO_ROOT) / rel / "config.schema.yaml"
            self.assertTrue(
                schema.is_file(),
                f"shim for {go_path!r} declared at {rel!r} but no config.schema.yaml found",
            )

    def test_shim_lookup_overrides_cache(self):
        """`_lookup_module_schema` returns the shim path when one is
        registered, regardless of what's in the cache."""
        from tasks.libs.otelcol_schema.bundle import REPO_ROOT, UPSTREAM_SHIMS, _lookup_module_schema

        any_shim_go_path = next(iter(UPSTREAM_SHIMS))
        result = _lookup_module_schema(
            any_shim_go_path,
            cache={},
            cache_root=Path("/nonexistent"),  # forces fallthrough on the non-shim path
            versions={},
        )
        self.assertIsNotNone(result)
        assert result is not None  # type-checker
        self.assertTrue(str(result).startswith(str(REPO_ROOT)))
        self.assertEqual(result.name, "config.schema.yaml")

    def test_shim_lookup_falls_through_for_unshimmed_paths(self):
        """If no shim is registered, the cache lookup runs as before."""
        from tasks.libs.otelcol_schema.bundle import _lookup_module_schema

        result = _lookup_module_schema(
            "example.com/not-a-real-module",
            cache={},
            cache_root=Path("/nonexistent"),
            versions={},
        )
        self.assertIsNone(result)


if __name__ == "__main__":
    unittest.main()
