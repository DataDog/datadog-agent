# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations

from typing import Union

from datadog_api_client.model_utils import (
    ModelNormal,
    cached_property,
    unset,
    UnsetType,
)


class SearchSLOResponseDataAttributesFacetsObjectInt(ModelNormal):
    @cached_property
    def openapi_types(_):
        return {
            "count": (int,),
            "name": (float,),
        }

    attribute_map = {
        "count": "count",
        "name": "name",
    }

    def __init__(self_, count: Union[int, UnsetType] = unset, name: Union[float, UnsetType] = unset, **kwargs):
        """
        Facet

        :param count: Count
        :type count: int, optional

        :param name: Facet
        :type name: float, optional
        """
        if count is not unset:
            kwargs["count"] = count
        if name is not unset:
            kwargs["name"] = name
        super().__init__(kwargs)
