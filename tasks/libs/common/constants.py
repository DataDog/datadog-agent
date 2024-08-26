DEFAULT_BRANCH = "main"
DEFAULT_INTEGRATIONS_CORE_BRANCH = "master"
GITHUB_ORG = "DataDog"
REPO_NAME = "datadog-agent"
GITHUB_REPO_NAME = f"{GITHUB_ORG}/{REPO_NAME}"
REPO_PATH = f"github.com/{GITHUB_REPO_NAME}"
ALLOWED_REPO_NON_NIGHTLY_BRANCHES = {"dev", "stable", "beta", "none"}
ALLOWED_REPO_NIGHTLY_BRANCHES = {"nightly", "oldnightly"}
ALLOWED_REPO_ALL_BRANCHES = ALLOWED_REPO_NON_NIGHTLY_BRANCHES.union(ALLOWED_REPO_NIGHTLY_BRANCHES)
AGENT_VERSION_CACHE_NAME = "agent-version.cache"

# Message templates for releasing tasks
# Defined here either because they're long and would make the code less legible,
# or because they're used multiple times.
RC_TAG_QUESTION_TEMPLATE = "The {} tag found is an RC tag: {}. Are you sure you want to use it?"
TAG_FOUND_TEMPLATE = "The {} tag is {}"
