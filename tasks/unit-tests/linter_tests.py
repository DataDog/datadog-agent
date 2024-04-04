import shutil
import unittest

import tasks.linter as linter


class TestIsGetParameterCall(unittest.TestCase):
    test_file = "test_linter.tmp"

    def tearDown(self):
        shutil.rmtree(self.test_file, ignore_errors=True)

    def test_no_get_param(self):
        with open(self.test_file, "w") as f:
            f.write("Hello World")
        matched = linter.is_get_parameter_call(self.test_file)
        self.assertIsNone(matched)

    def test_without_wrapper_no_env(self):
        with open(self.test_file, "w") as f:
            f.write(
                "API_KEY=$(aws ssm get-parameter --region us-east-1 --name ci.datadog-agent.datadog_api_key_org2 --with-decryption  --query Parameter.Value --out text)"
            )
        matched = linter.is_get_parameter_call(self.test_file)
        self.assertFalse(matched.with_wrapper)
        self.assertFalse(matched.with_env_var)

    def test_without_wrapper_with_env(self):
        with open(self.test_file, "w") as f:
            f.write(
                "  - export DD_API_KEY=$(aws ssm get-parameter --region us-east-1 --name $API_KEY_ORG2_SSM_NAME --with-decryption  --query Parameter.Value --out text"
            )
        matched = linter.is_get_parameter_call(self.test_file)
        self.assertFalse(matched.with_wrapper)
        self.assertTrue(matched.with_env_var)

    def test_with_wrapper_no_env(self):
        with open(self.test_file, "w") as f:
            f.write(
                "export DD_API_KEY=$($CI_PROJECT_DIR/tools/ci/aws_ssm_get_wrapper.sh ci.datadog-agent.datadog_api_key_org2)"
            )
        matched = linter.is_get_parameter_call(self.test_file)
        self.assertTrue(matched.with_wrapper)
        self.assertFalse(matched.with_env_var)

    def test_with_wrapper_with_env(self):
        with open(self.test_file, "w") as f:
            f.write("export DD_APP_KEY=$($CI_PROJECT_DIR/tools/ci/aws_ssm_get_wrapper.sh $APP_KEY_ORG2_SSM_NAME)")
        matched = linter.is_get_parameter_call(self.test_file)
        self.assertIsNone(matched)
