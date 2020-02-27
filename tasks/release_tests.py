import unittest
import release
import sys

class TestIsHigherMethod(unittest.TestCase):

    def _get_version(self, major, minor, patch, rc):
        return {
            "major": major,
            "minor": minor,
            "patch": patch,
            "rc": rc
        }

    def get_range(self, min, max):
        r = []
        for major in range(min,max):
            for minor in range(min,max):
                for patch in range(min,max):
                    for rc in range(min,max):
                        r.append(self._get_version(major, minor, patch, rc))
        return r

    def test(self):
        first_range = self.get_range(0,2)
        second_range = self.get_range(0,2)
        _1 = 0
        for range_1 in first_range:
            _2 = 0
            for range_2 in second_range:
                result = release._is_version_higher(range_1, range_2)
                range_1_str = release._stringify_version(range_1)
                range_2_str = release._stringify_version(range_2)
                if _1 > _2:
                    if not result and range_2["rc"] != 0:
                        sys.stdout.write('✗ ')
                        self.fail("{} < {}".format(range_1_str, range_2_str))
                    else:
                        sys.stdout.write('✓ ')
                    print("{} > {} = {}".format(range_1_str, range_2_str, result))
                _2 = _2 + 1
            _1 = _1 + 1

if __name__ == '__main__':
    unittest.main()
