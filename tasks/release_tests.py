import random
import re
import unittest

from . import release


class TestIsHigherMethod(unittest.TestCase):
    def _get_version(self, major, minor, patch, rc):
        return {"major": major, "minor": minor, "patch": patch, "rc": rc}

    def _get_random_version(self):
        return self._get_version(
            random.randint(0, 99), random.randint(0, 99), random.randint(0, 99), random.randint(0, 99)
        )

    def test_major_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                self._get_version(version["major"] + increment, version["minor"], version["patch"], version["rc"]),
            )
        )

    def test_major_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            release._is_version_higher(
                self._get_version(version["major"] + increment, version["minor"], version["patch"], version["rc"]),
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
            )
        )

    def test_minor_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                self._get_version(version["major"], version["minor"] + increment, version["patch"], version["rc"]),
            )
        )

    def test_minor_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"] + increment, version["patch"], version["rc"]),
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
            )
        )

    def test_patch_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                self._get_version(version["major"], version["minor"], version["patch"] + increment, version["rc"]),
            )
        )

    def test_patch_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"] + increment, version["rc"]),
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
            )
        )

    def test_rc_lower_than_release(self):
        version = self._get_random_version()
        self.assertFalse(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                self._get_version(version["major"], version["minor"], version["patch"], None),
            )
        )

    def test_release_higher_than_rc(self):
        version = self._get_random_version()
        self.assertTrue(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"], None),
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
            )
        )

    def test_rc_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"] + increment),
            )
        )

    def test_rc_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"] + increment),
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
            )
        )

    def test_equal(self):
        version = self._get_random_version()
        self.assertFalse(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
            )
        )

    def test_absent_patch_equal_zero(self):
        version = self._get_random_version()
        self.assertFalse(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], None, None),
                self._get_version(version["major"], version["minor"], 0, None),
            )
        )

    def test_absent_patch_less_than_any(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(
            release._is_version_higher(
                self._get_version(version["major"], version["minor"], version["patch"] + increment, None),
                self._get_version(version["major"], version["minor"], None, None),
            )
        )


class TestIsDictVersionField(unittest.TestCase):
    def test_non_version_fields_return_false(self):
        for key in ['FOO', 'BAR', 'WINDOWS_DDNPM_DRIVER', 'JMXFETCH_HASH']:
            self.assertFalse(release._is_dict_version_field(key))

    def test_version_fields_return_true(self):
        for key in ['FOO_VERSION', 'BAR_VERSION', 'JMXFETCH_VERSION']:
            self.assertTrue(release._is_dict_version_field(key))

    def test_version_fields_return_false_for_win_ddnpm_version(self):
        self.assertFalse(release._is_dict_version_field('WINDOWS_DDNPM_VERSION'))


class TestGetWindowsDDNPMReleaseJsonInfo(unittest.TestCase):
    test_version_re = re.compile(r'(v)?(\d+)[.](\d+)([.](\d+))?(-rc\.(\d+))?')
    test_release_json = {
        "nightly": {
            "WINDOWS_DDNPM_DRIVER": "attestation-signed",
            "WINDOWS_DDNPM_VERSION": "nightly-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "nightly-ddnpm-sha",
        },
        "nightly-a7": {
            "WINDOWS_DDNPM_DRIVER": "attestation-signed",
            "WINDOWS_DDNPM_VERSION": "nightly-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "nightly-ddnpm-sha",
        },
        "6.28.0-rc.3": {
            "WINDOWS_DDNPM_DRIVER": "release-signed",
            "WINDOWS_DDNPM_VERSION": "rc3-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "rc3-ddnpm-sha",
        },
        "7.28.0-rc.3": {
            "WINDOWS_DDNPM_DRIVER": "release-signed",
            "WINDOWS_DDNPM_VERSION": "rc3-ddnpm-version",
            "WINDOWS_DDNPM_SHASUM": "rc3-ddnpm-sha",
        },
    }

    def test_ddnpm_info_is_taken_from_nightly_on_first_rc(self):
        driver, version, shasum = release._get_windows_ddnpm_release_json_info(
            self.test_release_json, 7, self.test_version_re, True
        )

        self.assertEqual(driver, 'attestation-signed')
        self.assertEqual(version, 'nightly-ddnpm-version')
        self.assertEqual(shasum, 'nightly-ddnpm-sha')

    def test_ddnpm_info_is_taken_from_previous_rc_on_subsequent_rcs(self):
        driver, version, shasum = release._get_windows_ddnpm_release_json_info(
            self.test_release_json, 7, self.test_version_re, False
        )

        self.assertEqual(driver, 'release-signed')
        self.assertEqual(version, 'rc3-ddnpm-version')
        self.assertEqual(shasum, 'rc3-ddnpm-sha')


if __name__ == '__main__':
    unittest.main()
