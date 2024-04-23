import unittest

from invoke.exceptions import Exit

from tasks.flavor import AgentFlavor

from tasks.build_tags import DARWIN_EXCLUDED_TAGS, build_tags, get_build_tags


class TestBuildTags(unittest.TestCase):
    def test_default_tags(self):
        for flavor, targets in build_tags.items():
            for target, expected_tags in targets.items():
                with self.subTest(f"flavor: {flavor}, target: {target}"):
                    computed_tags = get_build_tags(build=target, flavor=flavor, platform='linux')
                    self.assertListEqual(computed_tags, sorted(expected_tags))

    def test_build_include(self):
        included_tags = "apm"
        computed_tags = get_build_tags(
            build="agent", flavor=AgentFlavor.base, build_include=included_tags, platform='linux'
        )
        expected_tags = ["apm"]
        self.assertListEqual(computed_tags, expected_tags)

    def test_build_exclude(self):
        included_tags = "apm,python"
        excluded_tags = "apm"
        computed_tags = get_build_tags(
            build="agent",
            flavor=AgentFlavor.base,
            build_include=included_tags,
            build_exclude=excluded_tags,
            platform='linux',
        )
        expected_tags = ["python"]
        self.assertListEqual(computed_tags, expected_tags)

    def test_flavor(self):
        computed_tags = get_build_tags(build="dogstatsd", flavor=AgentFlavor.dogstatsd, platform='linux')
        expected_tags = sorted(build_tags[AgentFlavor.dogstatsd]["dogstatsd"])
        self.assertListEqual(computed_tags, expected_tags)

    def test_platform(self):
        included_tags = ",".join(DARWIN_EXCLUDED_TAGS)
        computed_tags = get_build_tags(
            build="agent", flavor=AgentFlavor.base, build_include=included_tags, platform='darwin'
        )
        expected_tags = []
        self.assertListEqual(computed_tags, expected_tags)

    def test_raises_build(self):
        self.assertRaises(Exit, get_build_tags, build="nonexistant", flavor=AgentFlavor.base)


if __name__ == "__main__":
    unittest.main()
