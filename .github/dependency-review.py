import os
import requests

GITHUB_TOKEN = os.getenv("GITHUB_TOKEN")
DEPENDENCY_REVIEW_RESPONSE = requests.get(
    f"https://api.github.com/repos/datadog/datadog-agent/dependency-graph/compare/{os.getenv('BASE_REF')}...{os.getenv('HEAD_REF')}",
    headers={"Authorization": f"Bearer {GITHUB_TOKEN}"}
).json()
output = ""
output += "### Vulnerability Report\n\n"
# Loop through each entry from Dependency Review Response
for entry in DEPENDENCY_REVIEW_RESPONSE:
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
        output += f"\tAdvisory URL: {vulnerability['advisory_url']}\n"        
        output += f"\tSeverity: {vulnerability['severity']}\n"
        output += f"\tAdvisory Summary: {vulnerability['advisory_summary']}\n"
        output += f"\tCVE: https://nvd.nist.gov/vuln/detail/{CVE_INFO['cve_id']}\n"

print(output)

PR_NUMBER = os.getenv("PR_NUMBER")
API_URL = f"https://api.github.com/repos/{os.getenv('GITHUB_REPOSITORY')}/issues/{PR_NUMBER}/comments"
# Check if a comment already exists on the PR
existing_comment = None
existing_comments_response = requests.get(
    API_URL,
    headers={"Authorization": f"Bearer {GITHUB_TOKEN}"}
)
comment_exists = False
if existing_comments_response.status_code == 200:
    existing_comments = existing_comments_response.json()
    for comment in existing_comments:
        if comment["user"]["login"] == "github-actions[bot]" and "Vulnerability Report" in comment["body"]:
            comment_exists = True
            existing_comment = comment
            break

if comment_exists:
    API_URL = f"https://api.github.com/repos/{os.getenv('GITHUB_REPOSITORY')}/issues/comments/{existing_comment['id']}"
    update_response = requests.patch(
        API_URL,
        headers={"Authorization": f"Bearer {GITHUB_TOKEN}"},
        json={"body": output},
    )
    if update_response.status_code == 200:
        print("Comment updated successfully.")
    else:
        print("Failed to update comment.")
        print(response.text)
else:
    API_URL = f"https://api.github.com/repos/{os.getenv('GITHUB_REPOSITORY')}/issues/{PR_NUMBER}/comments"
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
