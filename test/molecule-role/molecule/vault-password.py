import os

# This file is used to generate the secret for ansible encrypt and decrypt SSH Key
# The CI job token will always be random
print(os.environ['CI_JOB_TOKEN'])
