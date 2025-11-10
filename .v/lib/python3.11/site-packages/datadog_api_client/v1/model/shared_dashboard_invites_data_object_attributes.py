# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2019-Present Datadog, Inc.
from __future__ import annotations

from typing import Union

from datadog_api_client.model_utils import (
    ModelNormal,
    cached_property,
    datetime,
    none_type,
    unset,
    UnsetType,
)


class SharedDashboardInvitesDataObjectAttributes(ModelNormal):
    @cached_property
    def openapi_types(_):
        return {
            "created_at": (datetime,),
            "email": (str,),
            "has_session": (bool,),
            "invitation_expiry": (datetime,),
            "session_expiry": (datetime, none_type),
            "share_token": (str,),
        }

    attribute_map = {
        "created_at": "created_at",
        "email": "email",
        "has_session": "has_session",
        "invitation_expiry": "invitation_expiry",
        "session_expiry": "session_expiry",
        "share_token": "share_token",
    }
    read_only_vars = {
        "created_at",
        "has_session",
        "invitation_expiry",
        "session_expiry",
        "share_token",
    }

    def __init__(
        self_,
        created_at: Union[datetime, UnsetType] = unset,
        email: Union[str, UnsetType] = unset,
        has_session: Union[bool, UnsetType] = unset,
        invitation_expiry: Union[datetime, UnsetType] = unset,
        session_expiry: Union[datetime, none_type, UnsetType] = unset,
        share_token: Union[str, UnsetType] = unset,
        **kwargs,
    ):
        """
        Attributes of the shared dashboard invitation

        :param created_at: When the invitation was sent.
        :type created_at: datetime, optional

        :param email: An email address that an invitation has been (or if used in invitation request, will be) sent to.
        :type email: str, optional

        :param has_session: Indicates whether an active session exists for the invitation (produced when a user clicks the link in the email).
        :type has_session: bool, optional

        :param invitation_expiry: When the invitation expires.
        :type invitation_expiry: datetime, optional

        :param session_expiry: When the invited user's session expires. null if the invitation has no associated session.
        :type session_expiry: datetime, none_type, optional

        :param share_token: The unique token of the shared dashboard that was (or is to be) shared.
        :type share_token: str, optional
        """
        if created_at is not unset:
            kwargs["created_at"] = created_at
        if email is not unset:
            kwargs["email"] = email
        if has_session is not unset:
            kwargs["has_session"] = has_session
        if invitation_expiry is not unset:
            kwargs["invitation_expiry"] = invitation_expiry
        if session_expiry is not unset:
            kwargs["session_expiry"] = session_expiry
        if share_token is not unset:
            kwargs["share_token"] = share_token
        super().__init__(kwargs)
