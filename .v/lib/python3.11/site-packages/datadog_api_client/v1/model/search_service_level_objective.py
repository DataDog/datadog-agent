# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations

from typing import Union, TYPE_CHECKING

from datadog_api_client.model_utils import (
    ModelNormal,
    cached_property,
    unset,
    UnsetType,
)


if TYPE_CHECKING:
    from datadog_api_client.v1.model.search_service_level_objective_data import SearchServiceLevelObjectiveData


class SearchServiceLevelObjective(ModelNormal):
    @cached_property
    def openapi_types(_):
        from datadog_api_client.v1.model.search_service_level_objective_data import SearchServiceLevelObjectiveData

        return {
            "data": (SearchServiceLevelObjectiveData,),
        }

    attribute_map = {
        "data": "data",
    }

    def __init__(self_, data: Union[SearchServiceLevelObjectiveData, UnsetType] = unset, **kwargs):
        """
        A service level objective data container.

        :param data: A service level objective ID and attributes.
        :type data: SearchServiceLevelObjectiveData, optional
        """
        if data is not unset:
            kwargs["data"] = data
        super().__init__(kwargs)
