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
    from datadog_api_client.v1.model.search_service_level_objective_attributes import (
        SearchServiceLevelObjectiveAttributes,
    )


class SearchServiceLevelObjectiveData(ModelNormal):
    @cached_property
    def openapi_types(_):
        from datadog_api_client.v1.model.search_service_level_objective_attributes import (
            SearchServiceLevelObjectiveAttributes,
        )

        return {
            "attributes": (SearchServiceLevelObjectiveAttributes,),
            "id": (str,),
            "type": (str,),
        }

    attribute_map = {
        "attributes": "attributes",
        "id": "id",
        "type": "type",
    }
    read_only_vars = {
        "id",
    }

    def __init__(
        self_,
        attributes: Union[SearchServiceLevelObjectiveAttributes, UnsetType] = unset,
        id: Union[str, UnsetType] = unset,
        type: Union[str, UnsetType] = unset,
        **kwargs,
    ):
        """
        A service level objective ID and attributes.

        :param attributes: A service level objective object includes a service level indicator, thresholds
            for one or more timeframes, and metadata ( ``name`` , ``description`` , and ``tags`` ).
        :type attributes: SearchServiceLevelObjectiveAttributes, optional

        :param id: A unique identifier for the service level objective object.

            Always included in service level objective responses.
        :type id: str, optional

        :param type: The type of the object, must be ``slo``.
        :type type: str, optional
        """
        if attributes is not unset:
            kwargs["attributes"] = attributes
        if id is not unset:
            kwargs["id"] = id
        if type is not unset:
            kwargs["type"] = type
        super().__init__(kwargs)
