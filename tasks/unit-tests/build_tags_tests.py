import sys
import unittest

from invoke.exceptions import Exit

from tasks.flavor import AgentFlavor

from ..build_tags import (
    ALL_TAGS,
    DARWIN_EXCLUDED_TAGS,
    LINUX_ONLY_TAGS,
    WINDOWS_32BIT_EXCLUDE_TAGS,
    WINDOWS_EXCLUDE_TAGS,
    build_tags,
    get_build_tags,
)


class TestBuildTags(unittest.TestCase):
    # Old versions of the build tags functions to verify that we didn't break functionality
    # To be removed after CI run
    def old_get_default_build_tags(self, build="agent", arch="x64", flavor=AgentFlavor.base, platform=sys.platform):
        include = build_tags.get(flavor).get(build)
        if include is None:
            print("Warning: unrecognized build type, no build tags included.", file=sys.stderr)
            include = set()

        return sorted(self.old_filter_incompatible_tags(include, arch=arch, platform=platform))

    def old_filter_incompatible_tags(self, include, arch="x64", platform=sys.platform):
        """
        Filter out tags incompatible with the platform.
        include can be a list or a set.
        """

        exclude = set()
        if not platform.startswith("linux"):
            exclude = exclude.union(LINUX_ONLY_TAGS)

        if platform == "win32":
            exclude = exclude.union(WINDOWS_EXCLUDE_TAGS)

        if platform == "darwin":
            exclude = exclude.union(DARWIN_EXCLUDED_TAGS)

        if platform == "win32" and arch == "x86":
            exclude = exclude.union(WINDOWS_32BIT_EXCLUDE_TAGS)

        return self.old_get_build_tags(include, exclude)

    def old_get_build_tags(self, include, exclude):
        """
        Build the list of tags based on inclusions and exclusions passed through
        the command line
        include and exclude can be lists or sets.
        """
        # Convert parameters to sets
        include = set(include)
        exclude = set(exclude)

        # filter out unrecognised tags
        known_include = ALL_TAGS.intersection(include)
        unknown_include = include - known_include
        for tag in unknown_include:
            print(f"Warning: unknown build tag '{tag}' was filtered out from included tags list.", file=sys.stderr)

        known_exclude = ALL_TAGS.intersection(exclude)
        unknown_exclude = exclude - known_exclude
        for tag in unknown_exclude:
            print(f"Warning: unknown build tag '{tag}' was filtered out from excluded tags list.", file=sys.stderr)

        return list(known_include - known_exclude)

    def test_regression_default_tags(self):
        # Regression test: we migrated all uses of get_default_build_tags to get_build_tags.
        # This checks that the use of get_build_tags without build_include and build_exclude
        # is equivalent to the use of the old get_default_build_tags code.
        for flavor, targets in build_tags.items():
            for target, _ in targets.items():
                with self.subTest(f"flavor: {flavor}, target: {target}"):
                    computed_tags = get_build_tags(build=target, flavor=flavor, platform='linux')
                    expected_tags = self.old_get_default_build_tags(build=target, flavor=flavor, platform='linux')
                    self.assertListEqual(computed_tags, sorted(expected_tags))

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
