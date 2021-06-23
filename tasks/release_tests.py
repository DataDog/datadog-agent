import re
import unittest

from . import release


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
