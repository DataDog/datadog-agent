from invoke import task

from tasks.libs.owners.parsing import search_owners


@task
def find_jobowners(_, job, owners_file=".gitlab/JOBOWNERS"):
    print(", ".join(search_owners(job, owners_file)))


@task
def find_codeowners(_, path, owners_file=".github/CODEOWNERS"):
    print(", ".join(search_owners(path, owners_file)))
