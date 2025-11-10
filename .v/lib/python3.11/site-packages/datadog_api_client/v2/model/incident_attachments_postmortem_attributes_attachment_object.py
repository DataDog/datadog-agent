# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations


from datadog_api_client.model_utils import (
    ModelNormal,
    cached_property,
)


class IncidentAttachmentsPostmortemAttributesAttachmentObject(ModelNormal):
    @cached_property
    def openapi_types(_):
        return {
            "document_url": (str,),
            "title": (str,),
        }

    attribute_map = {
        "document_url": "documentUrl",
        "title": "title",
    }

    def __init__(self_, document_url: str, title: str, **kwargs):
        """
        The postmortem attachment.

        :param document_url: The URL of this notebook attachment.
        :type document_url: str

        :param title: The title of this postmortem attachment.
        :type title: str
        """
        super().__init__(kwargs)

        self_.document_url = document_url
        self_.title = title
