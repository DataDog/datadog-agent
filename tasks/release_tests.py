import re
import unittest

from . import release


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
