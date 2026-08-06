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
)


def _payload():
    return build_tags.build_tags_codegen_payload()


class TestCodegenPayloadSchema(unittest.TestCase):
    REQUIRED_KEYS = {
        "common_tags",
        "unit_test_tags",
        "linux_only_tags",
        "windows_included_tags",
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
    def test_fips_includes_goexperiment_systemcrypto(self):
        self.assertIn("goexperiment.systemcrypto", _payload()["flavor_specific_tags"]["fips"])

    def test_base_excludes_requirefips(self):
        self.assertNotIn("requirefips", _payload()["flavor_specific_tags"]["base"])

    def test_wmi_in_windows_included_not_in_linux_only(self):
        payload = _payload()
        self.assertIn("wmi", payload["windows_included_tags"])
        self.assertNotIn("wmi", payload["linux_only_tags"])

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
