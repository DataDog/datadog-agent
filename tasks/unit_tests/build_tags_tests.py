import unittest

from tasks import build_tags
from tasks.build_tags import (
    ALL_TAGS,
    AUTO_TEST_TAGS,
    BASE_TEST_TAGS,
    COMMON_TAGS,
    DEP_ONLY_TAGS,
    GAZELLE_BUILD_TAGS,
    GAZELLE_EXTRA_TAGS,
    GAZELLE_OMIT_TAGS,
    TEST_FEATURE_TAGS,
    UNIT_TEST_TAGS,
    compute_build_tags_for_flavor,
)
from tasks.flavor import AgentFlavor


def _payload():
    return build_tags.build_tags_codegen_payload()


class TestCodegenPayloadSchema(unittest.TestCase):
    REQUIRED_KEYS = {
        "common_tags",
        "unit_test_tags",
        "linux_only_tags",
        "windows_excluded_tags",
        "darwin_excluded_tags",
        "flavor_specific_tags",
        "gazelle_build_tags",
    }

    def test_required_keys_present(self):
        self.assertEqual(set(_payload().keys()), self.REQUIRED_KEYS)

    def test_top_level_lists_are_sorted_and_unique(self):
        payload = _payload()
        for key in self.REQUIRED_KEYS - {"flavor_specific_tags"}:
            with self.subTest(key=key):
                value = payload[key]
                self.assertIsInstance(value, list)
                self.assertEqual(value, sorted(value), f"{key} not sorted")
                self.assertEqual(len(value), len(set(value)), f"{key} has duplicates")

    def test_flavor_specific_tags_are_sorted_and_unique(self):
        for flavor, tags in _payload()["flavor_specific_tags"].items():
            with self.subTest(flavor=flavor):
                self.assertIsInstance(tags, list)
                self.assertEqual(tags, sorted(tags))
                self.assertEqual(len(tags), len(set(tags)))


class TestCodegenPayloadData(unittest.TestCase):
    def test_recorder_flavor_is_base_agent_plus_recorder_tag(self):
        base = set(build_tags.get_default_build_tags(build="agent", flavor=AgentFlavor.base, platform="linux"))
        recorder = set(build_tags.get_default_build_tags(build="agent", flavor=AgentFlavor.recorder, platform="linux"))
        self.assertEqual(recorder, base | {"anomalydetection_recorder"})

    def test_recorder_flavor_keeps_other_binaries_at_base_tags(self):
        for build in ("trace-agent", "process-agent", "privateactionrunner"):
            with self.subTest(build=build):
                self.assertEqual(
                    build_tags.get_default_build_tags(build=build, flavor=AgentFlavor.recorder, platform="linux"),
                    build_tags.get_default_build_tags(build=build, flavor=AgentFlavor.base, platform="linux"),
                )

    def test_recorder_tag_survives_agent_build_tag_computation(self):
        tags = compute_build_tags_for_flavor(
            build="agent", flavor=AgentFlavor.recorder, build_include=None, build_exclude=None, platform="linux"
        )
        self.assertIn("anomalydetection_recorder", tags)

    def test_recorder_test_tag_sets_extend_agent(self):
        expected = build_tags.AGENT_RECORDER_TAGS.union(UNIT_TEST_TAGS).difference(build_tags.UNIT_TEST_EXCLUDED_TAGS)
        for build in ("lint", "unit-tests"):
            with self.subTest(build=build):
                recorder = build_tags.build_tags[AgentFlavor.recorder][build]
                self.assertEqual(recorder, expected)

    def test_fips_includes_goexperiment_systemcrypto(self):
        self.assertIn("goexperiment.systemcrypto", _payload()["flavor_specific_tags"]["fips"])

    def test_base_excludes_requirefips(self):
        self.assertNotIn("requirefips", _payload()["flavor_specific_tags"]["base"])

    def test_common_tags_disjoint_from_flavor_specific(self):
        # The .bzl/.go consumers compose flavor tag sets as
        # flavor_specific + common_tags + unit_test_tags; if common appears in
        # the flavor-specific list too we'd emit duplicates.
        payload = _payload()
        common = set(payload["common_tags"])
        for flavor, tags in payload["flavor_specific_tags"].items():
            with self.subTest(flavor=flavor):
                self.assertFalse(common & set(tags), f"{flavor} overlaps COMMON_TAGS")

    def test_gazelle_build_tags_matches_drift_formula(self):
        expected = sorted((ALL_TAGS - GAZELLE_OMIT_TAGS) | GAZELLE_EXTRA_TAGS)
        self.assertEqual(_payload()["gazelle_build_tags"], expected)
        self.assertEqual(sorted(GAZELLE_BUILD_TAGS), expected)

    def test_unit_test_tags_payload_matches_constant(self):
        self.assertEqual(_payload()["unit_test_tags"], sorted(UNIT_TEST_TAGS))

    def test_common_tags_payload_matches_constant(self):
        self.assertEqual(_payload()["common_tags"], sorted(COMMON_TAGS))

    def test_base_test_tags_are_minimal(self):
        self.assertEqual(BASE_TEST_TAGS, sorted(UNIT_TEST_TAGS))

    def test_auto_test_tags_exclude_dependency_only_tags(self):
        self.assertFalse(set(AUTO_TEST_TAGS) & DEP_ONLY_TAGS)

    def test_auto_test_tags_match_classification(self):
        expected = sorted(TEST_FEATURE_TAGS - DEP_ONLY_TAGS - UNIT_TEST_TAGS - build_tags.UNIT_TEST_EXCLUDED_TAGS)
        self.assertEqual(AUTO_TEST_TAGS, expected)


if __name__ == "__main__":
    unittest.main()
