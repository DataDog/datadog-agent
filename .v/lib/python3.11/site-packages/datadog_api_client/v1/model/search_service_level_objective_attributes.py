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
    from datadog_api_client.v1.model.slo_creator import SLOCreator
    from datadog_api_client.v1.model.slo_overall_statuses import SLOOverallStatuses
    from datadog_api_client.v1.model.search_slo_query import SearchSLOQuery
    from datadog_api_client.v1.model.slo_type import SLOType
    from datadog_api_client.v1.model.slo_status import SLOStatus
    from datadog_api_client.v1.model.search_slo_threshold import SearchSLOThreshold


class SearchServiceLevelObjectiveAttributes(ModelNormal):
    @cached_property
    def openapi_types(_):
        from datadog_api_client.v1.model.slo_creator import SLOCreator
        from datadog_api_client.v1.model.slo_overall_statuses import SLOOverallStatuses
        from datadog_api_client.v1.model.search_slo_query import SearchSLOQuery
        from datadog_api_client.v1.model.slo_type import SLOType
        from datadog_api_client.v1.model.slo_status import SLOStatus
        from datadog_api_client.v1.model.search_slo_threshold import SearchSLOThreshold

        return {
            "all_tags": ([str],),
            "created_at": (int,),
            "creator": (SLOCreator,),
            "description": (str, none_type),
            "env_tags": ([str],),
            "groups": ([str], none_type),
            "modified_at": (int,),
            "monitor_ids": ([int], none_type),
            "name": (str,),
            "overall_status": ([SLOOverallStatuses],),
            "query": (SearchSLOQuery,),
            "service_tags": ([str],),
            "slo_type": (SLOType,),
            "status": (SLOStatus,),
            "team_tags": ([str],),
            "thresholds": ([SearchSLOThreshold],),
        }

    attribute_map = {
        "all_tags": "all_tags",
        "created_at": "created_at",
        "creator": "creator",
        "description": "description",
        "env_tags": "env_tags",
        "groups": "groups",
        "modified_at": "modified_at",
        "monitor_ids": "monitor_ids",
        "name": "name",
        "overall_status": "overall_status",
        "query": "query",
        "service_tags": "service_tags",
        "slo_type": "slo_type",
        "status": "status",
        "team_tags": "team_tags",
        "thresholds": "thresholds",
    }
    read_only_vars = {
        "created_at",
        "modified_at",
    }

    def __init__(
        self_,
        all_tags: Union[List[str], UnsetType] = unset,
        created_at: Union[int, UnsetType] = unset,
        creator: Union[SLOCreator, none_type, UnsetType] = unset,
        description: Union[str, none_type, UnsetType] = unset,
        env_tags: Union[List[str], UnsetType] = unset,
        groups: Union[List[str], none_type, UnsetType] = unset,
        modified_at: Union[int, UnsetType] = unset,
        monitor_ids: Union[List[int], none_type, UnsetType] = unset,
        name: Union[str, UnsetType] = unset,
        overall_status: Union[List[SLOOverallStatuses], UnsetType] = unset,
        query: Union[SearchSLOQuery, none_type, UnsetType] = unset,
        service_tags: Union[List[str], UnsetType] = unset,
        slo_type: Union[SLOType, UnsetType] = unset,
        status: Union[SLOStatus, UnsetType] = unset,
        team_tags: Union[List[str], UnsetType] = unset,
        thresholds: Union[List[SearchSLOThreshold], UnsetType] = unset,
        **kwargs,
    ):
        """
        A service level objective object includes a service level indicator, thresholds
        for one or more timeframes, and metadata ( ``name`` , ``description`` , and ``tags`` ).

        :param all_tags: A list of tags associated with this service level objective.
            Always included in service level objective responses (but may be empty).
        :type all_tags: [str], optional

        :param created_at: Creation timestamp (UNIX time in seconds)

            Always included in service level objective responses.
        :type created_at: int, optional

        :param creator: The creator of the SLO
        :type creator: SLOCreator, none_type, optional

        :param description: A user-defined description of the service level objective.

            Always included in service level objective responses (but may be ``null`` ).
            Optional in create/update requests.
        :type description: str, none_type, optional

        :param env_tags: Tags with the ``env`` tag key.
        :type env_tags: [str], optional

        :param groups: A list of (up to 100) monitor groups that narrow the scope of a monitor service level objective.
            Included in service level objective responses if it is not empty.
        :type groups: [str], none_type, optional

        :param modified_at: Modification timestamp (UNIX time in seconds)

            Always included in service level objective responses.
        :type modified_at: int, optional

        :param monitor_ids: A list of monitor ids that defines the scope of a monitor service level
            objective.
        :type monitor_ids: [int], none_type, optional

        :param name: The name of the service level objective object.
        :type name: str, optional

        :param overall_status: calculated status and error budget remaining.
        :type overall_status: [SLOOverallStatuses], optional

        :param query: A metric-based SLO. **Required if type is metric**. Note that Datadog only allows the sum by aggregator
            to be used because this will sum up all request counts instead of averaging them, or taking the max or
            min of all of those requests.
        :type query: SearchSLOQuery, none_type, optional

        :param service_tags: Tags with the ``service`` tag key.
        :type service_tags: [str], optional

        :param slo_type: The type of the service level objective.
        :type slo_type: SLOType, optional

        :param status: Status of the SLO's primary timeframe.
        :type status: SLOStatus, optional

        :param team_tags: Tags with the ``team`` tag key.
        :type team_tags: [str], optional

        :param thresholds: The thresholds (timeframes and associated targets) for this service level
            objective object.
        :type thresholds: [SearchSLOThreshold], optional
        """
        if all_tags is not unset:
            kwargs["all_tags"] = all_tags
        if created_at is not unset:
            kwargs["created_at"] = created_at
        if creator is not unset:
            kwargs["creator"] = creator
        if description is not unset:
            kwargs["description"] = description
        if env_tags is not unset:
            kwargs["env_tags"] = env_tags
        if groups is not unset:
            kwargs["groups"] = groups
        if modified_at is not unset:
            kwargs["modified_at"] = modified_at
        if monitor_ids is not unset:
            kwargs["monitor_ids"] = monitor_ids
        if name is not unset:
            kwargs["name"] = name
        if overall_status is not unset:
            kwargs["overall_status"] = overall_status
        if query is not unset:
            kwargs["query"] = query
        if service_tags is not unset:
            kwargs["service_tags"] = service_tags
        if slo_type is not unset:
            kwargs["slo_type"] = slo_type
        if status is not unset:
            kwargs["status"] = status
        if team_tags is not unset:
            kwargs["team_tags"] = team_tags
        if thresholds is not unset:
            kwargs["thresholds"] = thresholds
        super().__init__(kwargs)
