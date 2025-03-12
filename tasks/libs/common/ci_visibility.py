import time
from contextlib import contextmanager
from dataclasses import dataclass
from typing import ClassVar

from tasks.libs.common.utils import running_in_gitlab_ci


@dataclass
class CIVisibilitySection:
    name: str
    start_time: float
    end_time: float
    tags: dict[str, str]
    measures: dict[str, float | int]
    sections: ClassVar = set()

    @staticmethod
    def send_all(ctx):
        if running_in_gitlab_ci() and CIVisibilitySection.sections:
            print('Sending CI visibility sections')
            for section in CIVisibilitySection.sections:
                section.send(ctx)

            CIVisibilitySection.sections.clear()

    @staticmethod
    def create(section_name, start_time: float, end_time: float, tags: dict[str, str] = None, measures: dict[str, float | int] = None):
        section = CIVisibilitySection(section_name, start_time, end_time, tags or {}, measures or {})
        CIVisibilitySection.sections.add(section)

        return section

    def send(self, ctx):
        def convert_time(t):
            return int(t * 1000)

        start_time = convert_time(self.start_time)
        # Ensure the section is at least 1 ms long to avoid errors
        end_time = max(convert_time(self.end_time), start_time + 1)

        tags = ''
        for key, value in list(self.tags.items()) + [('agent-custom-span', 'true')]:
            tags += f'--tags "{key}:{value}" '

        measures = ''
        for key, value in self.measures.items():
            measures += f'--measures "{key}:{value}" '

        ctx.run(f"datadog-ci span {tags}{measures}--name '{self.name}' --start-time {start_time} --end-time {end_time}")

    def __hash__(self):
        return hash((self.name, self.start_time, self.end_time))


# TODO: Tags / measures...
@contextmanager
def ci_visibility_section(section_name, ignore_on_error=False, force=False):
    """Creates a ci visibility span with the given name.

    Args:
        - ignore_on_error: If True, the section won't be created on error.
    """

    in_ci = running_in_gitlab_ci()
    if not in_ci and not force:
        yield
        return

    start_time = time.time()

    # TODO: Test cases etc...
    # TODO: Add error trace if exception is raised
    try:
        yield
    except:
        if ignore_on_error:
            return
    finally:
        end_time = time.time()

    # Create the section, this section will be sent when invoke exits
    CIVisibilitySection.create(section_name, start_time, end_time)


def ci_visibility_tag(ctx, name, value, level='job'):
    ctx.run(f'datadog-ci tag --tags "{name}:{value}" --level {level}')


def ci_visibility_measure(ctx, name, value, level='job'):
    ctx.run(f'datadog-ci measure --measures "{name}:{value}" --level {level}')
