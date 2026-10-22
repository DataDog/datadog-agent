#!/usr/bin/env python

import os
import re
import datetime

from azure.identity import ClientSecretCredential
from azure.mgmt.resource import ResourceManagementClient
from azure.core.exceptions import HttpResponseError

RESOURCE_GROUP_PREFIX=os.environ.get("RESOURCE_GROUP_PREFIX", "kitchen-dd-agent")
DEADLINE_DELTA=os.environ.get("DELETE_DELTA", 4) # delete all resource_groups older than 4 hours
DRY_RUN=os.environ.get("DRY_RUN", False)

print("Running Azure cleanup script with:")
print("  RESOURCE_GROUP_PREFIX: {}".format(RESOURCE_GROUP_PREFIX))
print("  DEADLINE_DELTA: {}".format(DEADLINE_DELTA))
print("  DRY_RUN: {}".format(DRY_RUN))

subscription_id = os.environ['ARM_SUBSCRIPTION_ID']
credentials = ClientSecretCredential(
    tenant_id=os.environ['ARM_TENANT_ID'],
    client_id=os.environ['ARM_CLIENT_ID'],
    client_secret=os.environ['ARM_CLIENT_SECRET'],
)

client = ResourceManagementClient(credentials, subscription_id)

time_limit = datetime.datetime.now() - datetime.timedelta(hours=DEADLINE_DELTA)
nb_resources_deleted = 0
for rg in client.resource_groups.list():
    if rg.name.startswith(RESOURCE_GROUP_PREFIX):
        started_time_match = re.match(".+-([0-9T]+)pl[0-9]+(-a[67])?", rg.name)
        if not started_time_match:
            # The resource group doesn't match the CI convention (eg. it ends with plnone).
            # Therefore, it must have been created manually and should be skipped.
            print("Skipping {}: not a resource group created by the CI".format(rg.name))
            continue
        started_time = started_time_match.groups()[0]
        if datetime.datetime.strptime(started_time, '%Y%m%dT%H%M%S') < time_limit:
            print("deleting {}".format(rg.name))
            if not DRY_RUN:
                # the delete is non blocking. The resource_groups will be
                # deleted in the next few minutes by azure. We ignore
                # ResourceGroupNotFound in case a previous job already trigger
                # a delete and it takes effect in the middle of the for loop.
                try:
                    client.resource_groups.begin_delete(rg.name)
                    nb_resources_deleted += 1
                except HttpResponseError as e:
                    if e.status_code != 404:
                        raise
                    else:
                        print("resource {} was already deleted".format(rg.name))
        else:
            print("keeping {}".format(rg.name))

print("{} resource_groups were deleted".format(nb_resources_deleted))
