import os
import requests

GITHUB_TOKEN = os.getenv("GITHUB_TOKEN")
ORG_NAME = "datadog"

API_URL = f"https://api.github.com/repos/{os.getenv('GITHUB_REPOSITORY')}/issues/{os.getenv('INPUT_PULL_REQUEST')}/comments"

REPOS = requests.get(
    f"https://api.github.com/repos/datadog/datadog-agent/dependency-graph/compare/{os.getenv('BASE_REF')}...{os.getenv('HEAD_REF')}",
    headers={"Authorization": f"Bearer {GITHUB_TOKEN}"}
).json()

output = ""
output += "### Vulnerability Report\n\n"
# Loop through each entry in the data
for entry in REPOS:
    output += f"Change Type: {entry['change_type']}\n"
    output += f"Manifest: {entry['manifest']}\n"
    output += f"Ecosystem: {entry['ecosystem']}\n"
    output += f"Name: {entry['name']}\n"
    output += f"Version: {entry['version']}\n"
    output += f"Package URL: {entry['package_url']}\n"
    output += f"Source Repository URL: {entry['source_repository_url']}\n"
    
    # Loop through vulnerabilities if present
    vulnerabilities = entry.get('vulnerabilities', [])
    for vulnerability in vulnerabilities:
        CVE_INFO = requests.get(
            f"https://api.github.com/advisories/{vulnerability['advisory_ghsa_id']}",
            headers={"Authorization": f"Bearer {GITHUB_TOKEN}"}
        ).json()
        output += f"\tSeverity: {vulnerability['severity']}\n"
        output += f"\tAdvisory Summary: {vulnerability['advisory_summary']}\n"
        output += f"\tCVE ID: {CVE_INFO['cve_id']}\n"
        output += f"\tAdvisory URL: {vulnerability['advisory_url']}\n\n"

print(output)

API_URL = f"https://api.github.com/repos/{os.getenv('GITHUB_REPOSITORY')}/issues/{os.getenv('PR_NUMBER')}/comments"
print(API_URL)
response = requests.post(
    API_URL,
    headers={"Authorization": f"Bearer {GITHUB_TOKEN}"},
    json={"body": output},
)

if response.status_code == 201:
    print("Comment created successfully.")
else:
    print("Failed to create comment.")
    print(response.text)
