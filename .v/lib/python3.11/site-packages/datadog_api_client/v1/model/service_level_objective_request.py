# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations

from typing import List, Union, TYPE_CHECKING

from datadog_api_client.model_utils import (
    ModelNormal,
    cached_property,
    none_type,
    unset,
    UnsetType,
)


if TYPE_CHECKING:
    from datadog_api_client.v1.model.service_level_objective_query import ServiceLevelObjectiveQuery
    from datadog_api_client.v1.model.slo_threshold import SLOThreshold
    from datadog_api_client.v1.model.slo_timeframe import SLOTimeframe
    from datadog_api_client.v1.model.slo_type import SLOType


class ServiceLevelObjectiveRequest(ModelNormal):
    @cached_property
    def openapi_types(_):
        from datadog_api_client.v1.model.service_level_objective_query import ServiceLevelObjectiveQuery
        from datadog_api_client.v1.model.slo_threshold import SLOThreshold
        from datadog_api_client.v1.model.slo_timeframe import SLOTimeframe
        from datadog_api_client.v1.model.slo_type import SLOType

        return {
            "description": (str, none_type),
            "groups": ([str],),
            "monitor_ids": ([int],),
            "name": (str,),
            "query": (ServiceLevelObjectiveQuery,),
            "tags": ([str],),
            "target_threshold": (float,),
            "thresholds": ([SLOThreshold],),
            "timeframe": (SLOTimeframe,),
            "type": (SLOType,),
            "warning_threshold": (float,),
        }

    attribute_map = {
        "description": "description",
        "groups": "groups",
        "monitor_ids": "monitor_ids",
        "name": "name",
        "query": "query",
        "tags": "tags",
        "target_threshold": "target_threshold",
        "thresholds": "thresholds",
        "timeframe": "timeframe",
        "type": "type",
        "warning_threshold": "warning_threshold",
    }

    def __init__(
        self_,
        name: str,
        thresholds: List[SLOThreshold],
        type: SLOType,
        description: Union[str, none_type, UnsetType] = unset,
        groups: Union[List[str], UnsetType] = unset,
        monitor_ids: Union[List[int], UnsetType] = unset,
        query: Union[ServiceLevelObjectiveQuery, UnsetType] = unset,
        tags: Union[List[str], UnsetType] = unset,
        target_threshold: Union[float, UnsetType] = unset,
        timeframe: Union[SLOTimeframe, UnsetType] = unset,
        warning_threshold: Union[float, UnsetType] = unset,
        **kwargs,
    ):
        """
        A service level objective object includes a service level indicator, thresholds
        for one or more timeframes, and metadata ( ``name`` , ``description`` , ``tags`` , etc.).

        :param description: A user-defined description of the service level objective.

            Always included in service level objective responses (but may be ``null`` ).
            Optional in create/update requests.
        :type description: str, none_type, optional

        :param groups: A list of (up to 100) monitor groups that narrow the scope of a monitor service level objective.

            Included in service level objective responses if it is not empty. Optional in
            create/update requests for monitor service level objectives, but may only be
            used when then length of the ``monitor_ids`` field is one.
        :type groups: [str], optional

        :param monitor_ids: A list of monitor IDs that defines the scope of a monitor service level
            objective. **Required if type is monitor**.
        :type monitor_ids: [int], optional

        :param name: The name of the service level objective object.
        :type name: str

        :param query: A metric-based SLO. **Required if type is metric**. Note that Datadog only allows the sum by aggregator
            to be used because this will sum up all request counts instead of averaging them, or taking the max or
            min of all of those requests.
        :type query: ServiceLevelObjectiveQuery, optional

        :param tags: A list of tags associated with this service level objective.
            Always included in service level objective responses (but may be empty).
            Optional in create/update requests.
        :type tags: [str], optional

        :param target_threshold: The target threshold such that when the service level indicator is above this
            threshold over the given timeframe, the objective is being met.
        :type target_threshold: float, optional

        :param thresholds: The thresholds (timeframes and associated targets) for this service level
            objective object.
        :type thresholds: [SLOThreshold]

        :param timeframe: The SLO time window options.
        :type timeframe: SLOTimeframe, optional

        :param type: The type of the service level objective.
        :type type: SLOType

        :param warning_threshold: The optional warning threshold such that when the service level indicator is
            below this value for the given threshold, but above the target threshold, the
            objective appears in a "warning" state. This value must be greater than the target
            threshold.
        :type warning_threshold: float, optional
        """
        if description is not unset:
            kwargs["description"] = description
        if groups is not unset:
            kwargs["groups"] = groups
        if monitor_ids is not unset:
            kwargs["monitor_ids"] = monitor_ids
        if query is not unset:
            kwargs["query"] = query
        if tags is not unset:
            kwargs["tags"] = tags
        if target_threshold is not unset:
            kwargs["target_threshold"] = target_threshold
        if timeframe is not unset:
            kwargs["timeframe"] = timeframe
        if warning_threshold is not unset:
            kwargs["warning_threshold"] = warning_threshold
        super().__init__(kwargs)

        self_.name = name
        self_.thresholds = thresholds
        self_.type = type
