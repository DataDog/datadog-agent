"""
test_builder contains all of the components needed to generate manual test cases.
This is made of of 3 parts:
 - Configuration
 - Test cases
 - Suites

Suites are made up of a collection of test cases and can take a configuration that is
passed into each test.
"""

from enum import Enum


class Platform(Enum):
    linux = 0
    windows = 1
    mac = 2


class LinuxConfig:
    platform = Platform.linux


class WindowsConfig:
    platform = Platform.windows


class MacConfig:
    platform = Platform.mac


class Suite:
    def __init__(self, config, tests):
        self.config = config
        self.tests = tests

    def build(self, renderDelegate):
        for test in self.tests:
            t = test()
            renderDelegate(t.name, t.render(self.config))


class TestCase:
    name = ""

    def __init__(self):
        self.steps = []

    def append(self, body):
        self.steps.append(body)

    def render(self, config):
        self.build(config)
        markdown = ""
        for step in self.steps:
            markdown += step + "\n\n"
        return markdown
