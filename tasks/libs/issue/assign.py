from collections import Counter
from pathlib import Path

from tasks.libs.ciproviders.github_api import get_github_teams
from tasks.libs.issue.model.constants import BASE_MODEL, MODEL, TEAMS
from tasks.libs.owners.parsing import most_frequent_agent_team, search_owners


def assign_with_model(issue):
    import torch
    from transformers import AutoModelForSequenceClassification, AutoTokenizer

    m = AutoModelForSequenceClassification.from_pretrained(
        f"{MODEL}", ignore_mismatched_sizes=True, local_files_only=True
    )
    m.eval()
    tokenizer = AutoTokenizer.from_pretrained(BASE_MODEL)
    inputs = tokenizer(
        f"{issue.title} {issue.body}".casefold(),
        padding='max_length',
        truncation=True,
        max_length=64,
        return_tensors='pt',
    )
    with torch.no_grad():
        outputs = m(**inputs)
    logits = outputs.logits
    proba = torch.softmax(logits, dim=1)
    predicted_class = torch.argmax(proba).item()
    confidence = proba[0][predicted_class].item()
    return TEAMS[torch.argmax(outputs.logits).item()], confidence


def assign_with_rules(issue, gh):
    owner = guess_from_labels(issue)
    if owner == 'triage':
        users = [user for user in issue.assignees if gh.is_organization_member(user)]
        teams = get_github_teams(users)
        owner = most_frequent_agent_team(teams)
    if owner == 'triage':
        commenters = [c.user for c in issue.get_comments() if gh.is_organization_member(c.user)]
        teams = get_github_teams(commenters)
        owner = most_frequent_agent_team(teams)
    if owner == 'triage':
        owner = guess_from_keywords(issue)
    return team_to_label(owner)


def guess_from_labels(issue):
    for label in issue.labels:
        if label.name.startswith("team/") and "triage" not in label.name:
            return label.name.split("/")[-1]
    return 'triage'


def guess_from_keywords(issue):
    text = f"{issue.title} {issue.body}".casefold().split()
    c = Counter(text)
    for word in c.most_common():
        team = simple_match(word[0])
        if team:
            return team
        team = file_match(word[0])
        if team:
            return team
    return "triage"


def simple_match(word):
    pattern_matching = {
        "agent-apm": ['apm', 'java', 'dotnet', 'ruby', 'trace'],
        "container-integrations": [
            'container',
            'pod',
            'kubernetes',
            'orchestrator',
            'docker',
            'k8s',
            'kube',
            'cluster',
            'kubelet',
            'helm',
        ],
        "agent-log-pipelines": ['logs', 'log-ag'],
        "agent-metric-pipelines": ['metric', 'statsd'],
        "agent-build-and-releases": ['omnibus', 'packaging', 'script'],
        "remote-config": ['installer', 'oci'],
        "agent-cspm": ['cspm'],
        "ebpf-platform": ['ebpf', 'system-prob', 'sys-prob'],
        "agent-security": ['security', 'vuln', 'security-agent'],
        "agent-runtimes": ['fips', 'payload', 'jmx', 'intake'],
        "agent-configuration": ['inventory', 'gohai'],
        "fleet": ['fleet', 'fleet-automation'],
        "opentelemetry": ['otel', 'opentelemetry'],
        "windows-products": ['windows', 'sys32', 'powershell'],
        "networks": ['tcp', 'udp', 'socket', 'network'],
        "serverless": ['serverless'],
        "integrations": ['integration', 'python', 'checks'],
    }
    for team, words in pattern_matching.items():
        if any(w in word for w in words):
            return team
    return None


def file_match(word):
    dd_folders = [
        'chocolatey',
        'cmd',
        'comp',
        'dev',
        'devenv',
        'docs',
        'internal',
        'omnibus',
        'pkg',
        'pkg-config',
        'rtloader',
        'tasks',
        'test',
        'tools',
    ]
    p = Path(word)
    if len(p.parts) > 1 and p.suffix:
        path_folder = next((f for f in dd_folders if f in p.parts), None)
        if path_folder:
            file = '/'.join(p.parts[p.parts.index(path_folder) :])
            return (
                search_owners(file, ".github/CODEOWNERS")[0].casefold().replace("@datadog/", "")
            )  # only return the first owner
    return None


def team_to_label(team):
    dico = {
        'apm-core-reliability-and-performance': "agent-apm",
        'universal-service-monitoring': "usm",
        'sdlc-security': "agent-security",
        'agent-all': "triage",
        'telemetry-and-analytics': "agent-apm",
        'fleet': "fleet-automation",
        'debugger': "dynamic-intrumentation",
        'agent-e2e-testing': "agent-e2e-test",
        'agent-integrations': "integrations",
        'asm-go': "agent-security",
    }
    return dico.get(team, team)
