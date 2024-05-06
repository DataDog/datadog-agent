import random
import unittest

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
