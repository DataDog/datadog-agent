import os

from tasks.libs.ciproviders.github_api import GithubAPI


def setup_ddqa(ctx):
    """
    Setup the environment for ddqa
    """
    config_file = ctx.run("ddqa config find", hide=True).stdout.strip()
    print(config_file)
    with open(config_file, "w") as config, open("tools/agent_QA/ddqa_template_config.toml") as template:
        config.write(template.read())
    print("coucou")
    ctx.run("ddqa config show", hide=False)
    print("cnous")
    ctx.run(f"cat {config_file}", hide=False)
    print("youhou")
    ctx.run(f"ddqa config set repos.datadog-agent.path {os.getcwd()}", hide=True)
    gh = GithubAPI()
    ctx.run("ddqa config set github.user github-actions[bot]", hide=True)
    ctx.run(f"ddqa config set github.token {gh._auth.token}", hide=True)
    ctx.run(f"ddqa config set jira.email {os.getenv('ATLASSIAN_USERNAME')}", hide=True)
    ctx.run(f"ddqa config set jira.token {os.getenv('ATLASSIAN_PASSWORD')}", hide=True)
    ctx.run("ddqa --auto sync", hide=False)
    ctx.run("ddqa config show", hide=False)


def get_labels(version):
    return f"-l {version} -l {version.qa_label()} -l ddqa"
