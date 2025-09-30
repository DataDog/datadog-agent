import unittest

from tasks.libs.types.types import JobDependency


class TestJobDependency(unittest.TestCase):
    def test_matches_simple_name(self):
        dep = JobDependency(job_name="build", tags=[])
        self.assertTrue(dep.matches("build"))
        self.assertFalse(dep.matches("test"))

    def test_matches_with_tags(self):
        dep = JobDependency(job_name="test", tags=[[("os", {"linux"}), ("arch", {"amd64"})]])
        # Correct job name and tags
        self.assertTrue(dep.matches("test: [linux, amd64]"))
        # Wrong job name
        self.assertFalse(dep.matches("build: [linux, amd64]"))
        # Wrong tag value
        self.assertFalse(dep.matches("test: [windows, amd64]"))
        # Missing tags
        self.assertFalse(dep.matches("test"))

    def test_matches_multiple_tagsets(self):
        dep = JobDependency(
            job_name="deploy",
            tags=[
                [("region", {"us-east-1"}), ("env", {"prod"})],
                [("region", {"eu-west-1"}), ("env", {"staging"})],
            ],
        )
        self.assertTrue(dep.matches("deploy: [us-east-1, prod]"))
        self.assertTrue(dep.matches("deploy: [eu-west-1, staging]"))
        self.assertFalse(dep.matches("deploy: [us-east-1, staging]"))

    def test_matches_invalid_tag_format(self):
        dep = JobDependency(job_name="foo", tags=[[("a", {"x"})]])
        with self.assertRaises(RuntimeError):
            dep.matches("foo: no_brackets")

    def test__match_tagset(self):
        dep = JobDependency(job_name="bar", tags=[])
        tagset = [("os", {"linux", "windows"}), ("arch", {"amd64"})]
        self.assertTrue(dep._match_tagset(tagset, ["linux", "amd64"]))
        self.assertFalse(dep._match_tagset(tagset, ["darwin", "amd64"]))
        self.assertFalse(dep._match_tagset(tagset, ["linux", "arm64"]))
        # Fewer job_tags than tagset
        self.assertFalse(dep._match_tagset(tagset, ["linux"]))
        # More job_tags than tagset (should ignore extras)
        self.assertTrue(dep._match_tagset(tagset, ["linux", "amd64", "extra"]))
