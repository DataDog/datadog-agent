import os
import random
import unittest
from unittest.mock import patch

from invoke import MockContext, Result

from tasks.libs.releasing.version import query_version
from tasks.libs.types.version import Version


class TestVersionComparison(unittest.TestCase):
    def _get_version(self, major, minor, patch, rc, devel):
        return Version(major, minor, patch=patch, rc=rc, devel=devel)

    def _get_random_version(self):
        return self._get_version(
            random.randint(0, 99),
            random.randint(0, 99),
            random.randint(0, 99),
            # For tests, rc must be non-0, as 0 signifies a release version, which would
            # break some tests like test_rc_higher and test_rc_lower
            random.randint(1, 99),
            random.choice([True, False]),
        )

    def test_major_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
            > self._get_version(version.major + increment, version.minor, version.patch, version.rc, version.devel)
        )

    def test_major_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major + increment, version.minor, version.patch, version.rc, version.devel)
            > self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
        )

    def test_minor_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
            > self._get_version(version.major, version.minor + increment, version.patch, version.rc, version.devel)
        )

    def test_minor_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major, version.minor + increment, version.patch, version.rc, version.devel)
            > self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
        )

    def test_patch_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
            > self._get_version(version.major, version.minor, version.patch + increment, version.rc, version.devel)
        )

    def test_patch_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch + increment, version.rc, version.devel)
            > self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
        )

    def test_rc_lower_than_release(self):
        version = self._get_random_version()
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
            > self._get_version(version.major, version.minor, version.patch, None, version.devel)
        )

    def test_release_higher_than_rc(self):
        version = self._get_random_version()
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch, None, version.devel)
            > self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
        )

    def test_rc_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
            > self._get_version(version.major, version.minor, version.patch, version.rc + increment, version.devel)
        )

    def test_rc_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch, version.rc + increment, version.devel)
            > self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
        )

    def test_equal(self):
        version = self._get_random_version()
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
            > self._get_version(version.major, version.minor, version.patch, version.rc, version.devel)
        )

    def test_absent_patch_equal_zero(self):
        version = self._get_random_version()
        self.assertFalse(
            self._get_version(version.major, version.minor, None, None, version.devel)
            > self._get_version(version.major, version.minor, 0, None, version.devel)
        )

    def test_absent_patch_less_than_any(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch + increment, None, version.devel)
            > self._get_version(version.major, version.minor, None, None, version.devel)
        )

    def test_devel_less_than_any(self):
        version = self._get_random_version()
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch, None, False)
            > self._get_version(version.major, version.minor, version.patch, None, True)
        )

    def test_devel_less_than_rc(self):
        version = self._get_random_version()
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch, version.rc, False)
            > self._get_version(version.major, version.minor, version.patch, None, True)
        )

    def test_devel_equal(self):
        version = self._get_random_version()
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch, None, True)
            == self._get_version(version.major, version.minor, version.patch, None, True)
        )


class TestNonDevelVersion(unittest.TestCase):
    version = Version(major=1, minor=0, devel=True)

    def test_non_devel_version(self):
        new_version = self.version.non_devel_version()
        expected_version = Version(major=1, minor=0)  # 1.0.0

        self.assertEqual(new_version, expected_version)


class TestNextVersion(unittest.TestCase):
    version = Version(major=1, minor=0)

    def test_next_version_major(self):
        new_version = self.version.next_version(bump_major=True)
        expected_version = Version(major=2, minor=0)

        self.assertEqual(new_version, expected_version)

    def test_next_version_minor(self):
        new_version = self.version.next_version(bump_minor=True)
        expected_version = Version(major=1, minor=1)

        self.assertEqual(new_version, expected_version)

    def test_next_version_patch(self):
        new_version = self.version.next_version(bump_patch=True)
        expected_version = Version(major=1, minor=0, patch=1)

        self.assertEqual(new_version, expected_version)

    def test_next_version_major_rc(self):
        new_version = self.version.next_version(bump_major=True, rc=True)
        expected_version = Version(major=2, minor=0, rc=1)

        self.assertEqual(new_version, expected_version)

    def test_next_version_minor_rc(self):
        new_version = self.version.next_version(bump_minor=True, rc=True)
        expected_version = Version(major=1, minor=1, rc=1)

        self.assertEqual(new_version, expected_version)

    def test_next_version_patch_rc(self):
        new_version = self.version.next_version(bump_patch=True, rc=True)
        expected_version = Version(major=1, minor=0, patch=1, rc=1)

        self.assertEqual(new_version, expected_version)

    def test_next_version_rc(self):
        version = self.version.next_version(bump_patch=True, rc=True)  # 1.0.1-rc.1
        new_version = version.next_version(rc=True)
        expected_version = Version(major=1, minor=0, patch=1, rc=2)

        self.assertEqual(new_version, expected_version)

    def test_next_version_promote_rc(self):
        version = self.version.next_version(bump_patch=True, rc=True)  # 1.0.1-rc.1
        new_version = version.next_version(rc=False)
        expected_version = Version(major=1, minor=0, patch=1)

        self.assertEqual(new_version, expected_version)


class TestQueryVersion(unittest.TestCase):
    @patch.dict(os.environ, {"BUCKET_BRANCH": "dev"}, clear=True)
    def test_on_dev_bucket(self):
        major_version = "7"
        c = MockContext(
            run={
                r'git describe --tags --candidates=50 --match "7\.*" --abbrev=7': Result(
                    "7.54.0-dbm-mongo-0.1-163-g315e3a2"
                )
            }
        )
        v, p, c, g, _ = query_version(c, major_version)
        self.assertEqual(v, "7.54.0")
        self.assertEqual(p, "dbm-mongo-0.1")
        self.assertEqual(c, 163)
        self.assertEqual(g, "315e3a2")

    @patch.dict(os.environ, {"BUCKET_BRANCH": "nightly"}, clear=True)
    def test_on_nightly_bucket(self):
        major_version = "7"
        c = MockContext(
            run={
                "git rev-parse --abbrev-ref HEAD": Result("main"),
                rf"git tag --list --merged main | grep -E '^{major_version}\.[0-9]+\.[0-9]+(-rc.*|-devel.*)?$' | sort -rV | head -1": Result(
                    "7.55.0-devel"
                ),
                'git describe --tags --candidates=50 --match "7.55.0-devel" --abbrev=7': Result(
                    "7.55.0-devel-543-g315e3a2"
                ),
            }
        )
        v, p, c, g, _ = query_version(c, major_version)
        self.assertEqual(v, "7.55.0")
        self.assertEqual(p, "devel")
        self.assertEqual(c, 543)
        self.assertEqual(g, "315e3a2")

    def test_on_release(self):
        major_version = "7"
        c = MockContext(
            run={
                "git rev-parse --abbrev-ref HEAD": Result("7.55.x"),
                rf"git tag --list --merged 7.55.x | grep -E '^{major_version}\.[0-9]+\.[0-9]+(-rc.*|-devel.*)?$' | sort -rV | head -1": Result(
                    "7.55.0-devel"
                ),
                'git describe --tags --candidates=50 --match "7.55.0-devel" --abbrev=7': Result(
                    "7.55.0-devel-543-g315e3a2"
                ),
            }
        )
        v, p, c, g, _ = query_version(c, major_version, release=True)
        self.assertEqual(v, "7.55.0")
        self.assertEqual(p, "devel")
        self.assertEqual(c, 543)
        self.assertEqual(g, "315e3a2")
