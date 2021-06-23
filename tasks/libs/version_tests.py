import random
import unittest

from .version import Version


class TestVersionComparison(unittest.TestCase):
    def _get_version(self, major, minor, patch, rc):
        return Version(major, minor, patch=patch, rc=rc)

    def _get_random_version(self):
        return self._get_version(
            random.randint(0, 99), random.randint(0, 99), random.randint(0, 99), random.randint(0, 99)
        )

    def test_major_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc)
            > self._get_version(version.major + increment, version.minor, version.patch, version.rc)
        )

    def test_major_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major + increment, version.minor, version.patch, version.rc)
            > self._get_version(version.major, version.minor, version.patch, version.rc)
        )

    def test_minor_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc)
            > self._get_version(version.major, version.minor + increment, version.patch, version.rc)
        )

    def test_minor_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major, version.minor + increment, version.patch, version.rc)
            > self._get_version(version.major, version.minor, version.patch, version.rc)
        )

    def test_patch_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc)
            > self._get_version(version.major, version.minor, version.patch + increment, version.rc)
        )

    def test_patch_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch + increment, version.rc)
            > self._get_version(version.major, version.minor, version.patch, version.rc)
        )

    def test_rc_lower_than_release(self):
        version = self._get_random_version()
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc)
            > self._get_version(version.major, version.minor, version.patch, None)
        )

    def test_release_higher_than_rc(self):
        version = self._get_random_version()
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch, None)
            > self._get_version(version.major, version.minor, version.patch, version.rc)
        )

    def test_rc_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc)
            > self._get_version(version.major, version.minor, version.patch, version.rc + increment)
        )

    def test_rc_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch, version.rc + increment)
            > self._get_version(version.major, version.minor, version.patch, version.rc)
        )

    def test_equal(self):
        version = self._get_random_version()
        self.assertFalse(
            self._get_version(version.major, version.minor, version.patch, version.rc)
            > self._get_version(version.major, version.minor, version.patch, version.rc)
        )

    def test_absent_patch_equal_zero(self):
        version = self._get_random_version()
        self.assertFalse(
            self._get_version(version.major, version.minor, None, None)
            > self._get_version(version.major, version.minor, 0, None)
        )

    def test_absent_patch_less_than_any(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            self._get_version(version.major, version.minor, version.patch + increment, None)
            > self._get_version(version.major, version.minor, None, None)
        )


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


if __name__ == '__main__':
    unittest.main()
