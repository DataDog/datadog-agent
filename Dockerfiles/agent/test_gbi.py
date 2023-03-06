#!/opt/datadog-agent/embedded/bin/python

import os
import os.path
import subprocess
import unittest
import yaml

CIS_FILE = '/tmp/cis.yaml'
COMMAND_TEST = 'commandTests'
FILE_EXISTENCE_TESTS = 'fileExistenceTests'


def create_tests(func_name, file, testCategory):
    def decorator(cls):
        func = getattr(cls, func_name)
        with open(file, 'r') as f:
            data = yaml.safe_load(f)[testCategory]
            for _, params in enumerate(data):

                def tmp(params=params):
                    def wrapper(self):
                        return func(self, **params)

                    return wrapper

                test_name = 'test_' + params['name'].replace(" ", "_").lower()
                setattr(cls, test_name, tmp())
        setattr(cls, func_name, None)
        return cls

    return decorator


@create_tests('test_file_existence', CIS_FILE, FILE_EXISTENCE_TESTS)
@create_tests('test_commands', CIS_FILE, COMMAND_TEST)
class TestGoldenBaseImage(unittest.TestCase):
    def test_file_existence(self, path, shouldExist, **kwargs):
        if shouldExist:
            self.assertTrue(os.path.isfile(path), path + " should be present")
        else:
            self.assertFalse(os.path.isfile(path), path + " should NOT be present")

    def test_commands(self, command, args, exitCode=0, expectedOutput=None, **kwargs):
        full_command = [command] + args
        p = subprocess.run(full_command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(exitCode, p.returncode)
        if expectedOutput is not None:
            expectedOutput = ''.join(expectedOutput)
            output = p.stdout.decode().strip()
            self.assertEqual(expectedOutput, output)


if __name__ == "__main__":
    unittest.main()
