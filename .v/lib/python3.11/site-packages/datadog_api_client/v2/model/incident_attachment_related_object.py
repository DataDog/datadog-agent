# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations


from datadog_api_client.model_utils import (
    ModelSimple,
    cached_property,
)

from typing import ClassVar


class IncidentAttachmentRelatedObject(ModelSimple):
    """
    The object related to an incident attachment.

    :param value: If omitted defaults to "users". Must be one of ["users"].
    :type value: str
    """

    allowed_values = {
        "users",
    }
    USERS: ClassVar["IncidentAttachmentRelatedObject"]

    @cached_property
    def openapi_types(_):
        return {
            "value": (str,),
        }


IncidentAttachmentRelatedObject.USERS = IncidentAttachmentRelatedObject("users")
