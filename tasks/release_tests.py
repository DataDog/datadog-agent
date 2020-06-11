import unittest
import release
import random

class TestIsHigherMethod(unittest.TestCase):

    def _get_version(self, major, minor, patch, rc):
        return {
            "major": major,
            "minor": minor,
            "patch": patch,
            "rc": rc
        }

    def _get_random_version(self):
        return self._get_version(random.randint(0, 99), random.randint(0, 99), random.randint(0, 99), random.randint(0, 99))

    def test_major_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                                                    self._get_version(version["major"] + increment, version["minor"], version["patch"], version["rc"])))

    def test_major_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(release._is_version_higher(self._get_version(version["major"] + increment, version["minor"], version["patch"], version["rc"]),
                                                   self._get_version(version["major"], version["minor"], version["patch"], version["rc"])))

    def test_minor_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                                                    self._get_version(version["major"], version["minor"] + increment, version["patch"], version["rc"])))

    def test_minor_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(release._is_version_higher(self._get_version(version["major"], version["minor"] + increment, version["patch"], version["rc"]),
                                                   self._get_version(version["major"], version["minor"], version["patch"], version["rc"])))

    def test_patch_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                                                    self._get_version(version["major"], version["minor"], version["patch"] + increment, version["rc"])))

    def test_patch_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"] + increment, version["rc"]),
                                                self._get_version(version["major"], version["minor"], version["patch"], version["rc"])))

    def test_rc_lower_than_release(self):
        version = self._get_random_version()
        self.assertFalse(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                                                    self._get_version(version["major"], version["minor"], version["patch"], None)))

    def test_release_higher_than_rc(self):
        version = self._get_random_version()
        self.assertTrue(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"], None),
                                                   self._get_version(version["major"], version["minor"], version["patch"], version["rc"])))

    def test_rc_lower(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertFalse(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                                                    self._get_version(version["major"], version["minor"], version["patch"], version["rc"] + increment)))

    def test_rc_higher(self):
        version = self._get_random_version()
        increment = random.randint(1, 99)
        self.assertTrue(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"], version["rc"] + increment),
                                                   self._get_version(version["major"], version["minor"], version["patch"], version["rc"])))

    def test_equal(self):
            version = self._get_random_version()
            self.assertFalse(release._is_version_higher(self._get_version(version["major"], version["minor"], version["patch"], version["rc"]),
                                                        self._get_version(version["major"], version["minor"], version["patch"], version["rc"])))

if __name__ == '__main__':
    unittest.main()
