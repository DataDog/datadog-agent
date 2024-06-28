import os
from datetime import timedelta

from tasks.libs.owners.parsing import list_owners


def _stringify_config(config_dict):
    """
    Takes a config dict of the following form:
    {
        "xxx_VERSION": Version(major: x, minor: y, patch: z, rc: t, prefix: "pre"),
        "xxx_HASH": "hashvalue",
        ...
    }

    and transforms all VERSIONs into their string representation (using the Version object's __str__).
    """
    return {key: str(value) for key, value in config_dict.items()}


def nightly_entry_for(agent_major_version):
    if agent_major_version == 6:
        return "nightly"
    return f"nightly-a{agent_major_version}"


def release_entry_for(agent_major_version):
    return f"release-a{agent_major_version}"


def create_release_page(version, freeze_date):
    username = os.environ['ATLASSIAN_USERNAME']
    password = os.environ['ATLASSIAN_PASSWORD']
    space_key = "agent"
    parent_page_id = "2244936127"
    # Make the POST request to create the page
    domain = "https://datadoghq.atlassian.net/wiki"
    from atlassian import Confluence

    confluence = Confluence(url=domain, username=username, password=password)
    page_title = f"Agent {version}"
    teams = get_releasing_teams()
    page = confluence.create_page(
        space=space_key,
        title=page_title,
        body=create_release_table(version, freeze_date, teams),
        parent_id=parent_page_id,
        editor="v2",
    )
    release_page = {"id": page["id"], "url": f"{domain}{page['_links']['webui']}"}
    confluence.create_page(
        space=space_key,
        title=f"{page_title} Notes",
        body=create_release_notes(freeze_date, teams),
        parent_id=release_page["id"],
    )
    return release_page


def get_release_page_info(version):
    username = os.environ['ATLASSIAN_USERNAME']
    password = os.environ['ATLASSIAN_PASSWORD']
    space_key = "agent"
    domain = "https://datadoghq.atlassian.net/wiki"
    from atlassian import Confluence

    c = Confluence(url=domain, username=username, password=password)
    page = c.get_page_by_title(space_key, f"Agent {version}", expand="body.storage")
    return f"{domain}{page['_links']['webui']}", parse_table(page['body']['storage']['value'])


def parse_table(data):
    from bs4 import BeautifulSoup

    soup = BeautifulSoup(data, 'lxml')

    # Find the table containing "Release managers"
    table = soup.find('table')
    rows = table.find_all('tr')
    rm_start_row = next(row for row in rows if row.find_all('td')[0].text == 'Release managers')
    start = rows.index(rm_start_row)
    for row in rows[start:]:
        cells = row.find_all('td')
        if len(cells) > 1 and len(cells[-1].find_all('ri:user')) == 0:
            yield cells[-2].text


def get_releasing_teams():
    non_releasing_teams = {
        'telemetry-and-analytics',
        'documentation',
        'software-integrity-and-trust',
        'single-machine-performance',
        'agent-all',
        'apm-core-reliability-and-performance',
        'debugger',
        'asm-go',
        'agent-e2e-testing',
        'serverless',
        'agent-platform',
        'agent-release-management',
        'container-ecosystems',
    }
    owners = set(list_owners())
    return sorted(owners - non_releasing_teams)


def create_release_table(version, freeze_date, teams):
    from yattag import Doc

    doc, tag, text, line = Doc().ttl()
    line('h2', 'Summary')
    with tag(
        'table',
        ('data-table-width', "760"),
        ('data-layout', "default"),
    ):
        with tag('colgroup'):
            for _ in range(3):
                with tag('col', style="width: 226.67px;"):
                    pass
        with tag('tbody'):
            with tag('tr'):
                with tag('td'), tag('p'):
                    text('Status')
                with tag('td', colspan="2"), tag('p', style="text-align: center;"):
                    with tag('ac:structured-macro', ('ac:name', "status"), ('ac:schema-version', "1")):
                        with tag('ac:parameter', ('ac:name', "title")):
                            text('QA')
                        with tag('ac:parameter', ('ac:name', "colour")):
                            text('Purple')
            with tag('tr'):
                with tag('td'), tag('p'):
                    text('Release date')
                with (
                    tag('td', colspan="2"),
                    tag('p', style="text-align: center;"),
                    tag('time', datetime=f"{freeze_date + timedelta(days=26)}"),
                ):
                    pass
            with tag('tr'):
                with tag('td'), tag('p'):
                    text('Release notes')
                with tag('td', colspan="2"), tag('p', style="text-align: center;"):
                    with tag('a', href=f"https://github.com/DataDog/datadog-agent/releases/tag/{version}"):
                        text(f'https://github.com/DataDog/datadog-agent/releases/tag/{version}')
            with tag('tr'):
                with tag('td'), tag('p'):
                    text('Code freeze date')
                with (
                    tag('td', colspan="2"),
                    tag('p', style="text-align: center;"),
                    tag('time', datetime=f"{freeze_date}"),
                ):
                    pass
            with tag('tr'):
                with tag('td'), tag('p'):
                    text('Release coordinator')
                with tag('td', colspan="2"), tag('p', style="text-align: center;"):
                    with tag('ac:link'):
                        with tag('ri:user', ('ri:account-id', "61142ccffc68c1006952fe23")):
                            pass
            with tag('tr'):
                with tag('td', rowspan=str(len(teams))), tag('p'):
                    text('Release managers')
                    with tag('td'), tag('p'):
                        text(teams[0])
                    with tag('td'), tag('p', style="text-align: center;"):
                        pass
            for team in teams[1:]:
                with tag('tr'):
                    with tag('td'), tag('p'):
                        text(team)
                    with tag('td'), tag('p', style="text-align: center;"):
                        pass

    line('h2', 'Major changes')
    with tag('table', ('data-table-width', "760"), ('data-layout', "default")):
        with tag('colgroup'), tag('col', style="width: 760.0px;"):
            pass
        with tag('tbody'), tag('tr'), tag('td'), tag('p'):
            pass
    return doc.getvalue()


def create_release_notes(freeze_date, teams):
    from yattag import Doc

    doc, tag, text, line = Doc().ttl()
    milestones = {
        '"Code freeze"': freeze_date,
        '"RC.1 built"': freeze_date + timedelta(days=3),
        '"Staging deployment"': freeze_date + timedelta(days=5),
        '"Prod deployment start"': freeze_date + timedelta(days=11),
        '"Full prod deployment"': freeze_date + timedelta(days=20),
        '"Release"': freeze_date + timedelta(days=26),
    }

    line('h2', 'Schedule')
    for i, item in enumerate(milestones.items()):
        milestone, date = item
        with tag('p'):
            text(f'Milestone {i} - {milestone} - ')
            with tag('time', datetime=str(date)):
                pass

    line('h2', 'Timeline')
    for i, milestone in enumerate(milestones.keys()):
        line('p', f'Milestone {i} - {milestone} - ')

    line('h2', 'Comments')
    with tag(
        'table',
        ('data-table-width', "1220"),
        ('data-layout', "default"),
        ('ac:local-id', "a9ca104f-228e-4d8a-bb81-07f928682bb6"),
    ):
        with tag('colgroup'):
            with tag('col', style="width: 477.0px;"):
                pass
            with tag('col', style="width: 743.0px;"):
                pass
        with tag('tbody'):
            for team in teams:
                with tag('tr'):
                    with tag('td'), tag('p'):
                        text(team)
                    with tag('td'), tag('p'):
                        pass

    return doc.getvalue()
