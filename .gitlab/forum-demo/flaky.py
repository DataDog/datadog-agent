"""Fail CodeSync-triggered attempts and pass user-triggered attempts."""

import os
import sys

trigger_user = os.environ["GITLAB_USER_LOGIN"]
print(f"Job triggered by {trigger_user}.")

if trigger_user == "codesync":
    print("FAIL: synthetic transient failure on a CodeSync-triggered attempt.")
    sys.exit(1)

print("PASS: user-triggered attempt succeeded.")
