# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations

from typing import TYPE_CHECKING

from datadog_api_client.model_utils import (
    ModelNormal,
    cached_property,
)


if TYPE_CHECKING:
    from datadog_api_client.v1.model.shared_dashboard_invites_data_object_attributes import (
        SharedDashboardInvitesDataObjectAttributes,
    )
    from datadog_api_client.v1.model.dashboard_invite_type import DashboardInviteType


class SharedDashboardInvitesDataObject(ModelNormal):
    @cached_property
    def openapi_types(_):
        from datadog_api_client.v1.model.shared_dashboard_invites_data_object_attributes import (
            SharedDashboardInvitesDataObjectAttributes,
        )
        from datadog_api_client.v1.model.dashboard_invite_type import DashboardInviteType

        return {
            "attributes": (SharedDashboardInvitesDataObjectAttributes,),
            "type": (DashboardInviteType,),
        }

    attribute_map = {
        "attributes": "attributes",
        "type": "type",
    }

    def __init__(self_, attributes: SharedDashboardInvitesDataObjectAttributes, type: DashboardInviteType, **kwargs):
        """
        Object containing the information for an invitation to a shared dashboard.

        :param attributes: Attributes of the shared dashboard invitation
        :type attributes: SharedDashboardInvitesDataObjectAttributes

        :param type: Type for shared dashboard invitation request body.
        :type type: DashboardInviteType
        """
        super().__init__(kwargs)

        self_.attributes = attributes
        self_.type = type
