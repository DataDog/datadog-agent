import os
from datetime import timedelta

import requests

from tasks.libs.owners.parsing import list_owners


def create_release_page(version, freeze_date):
    username = os.environ['ATLASSIAN_USERNAME']
    password = os.environ['ATLASSIAN_PASSWORD']
    auth = requests.auth.HTTPBasicAuth(username, password)
    space_key = "agent"
    parent_page_id = "2244936127"
    # Make the POST request to create the page
    domain = "https://datadoghq.atlassian.net/wiki"
    url = f"{domain}/rest/api/content"
    headers = {"Content-Type": "application/json"}
    page_title = f"Agent {version}"
    teams = get_releasing_teams()
    data = get_page_data(page_title, parent_page_id, space_key, create_release_table(version, freeze_date, teams))
    response = requests.post(url=url, headers=headers, auth=auth, json=data)
    response.raise_for_status()
    release_page = {"id": response.json()["id"], "url": f"{domain}{response.json()['_links']['webui']}"}
    note_data = get_page_data(
        f"{page_title} Notes", release_page["id"], space_key, create_release_notes(freeze_date, teams)
    )
    response = requests.post(url=url, headers=headers, auth=auth, json=note_data)
    response.raise_for_status()
    return release_page


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
    return list(owners - non_releasing_teams)


def get_page_data(title, parent_page_id, space_key, body):
    return {
        'type': 'page',
        'title': title,
        'ancestors': [{'id': parent_page_id}],
        'space': {'key': space_key},
        'status': 'current',
        'body': {
            'storage': {
                'representation': 'storage',
                'value': body,
            }
        },
        # The following metadata is required to view the page properly, see https://jira.atlassian.com/browse/CONFCLOUD-68057
        "metadata": {"properties": {"editor": {"key": "editor", "value": "v2"}}},
    }


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
