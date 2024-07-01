---
title: CSM Threats Events Formats
description: JSON schema documentation of the CSM Threats backend event
disable_edit: true
---



When activity matches a [Cloud Security Management Threats][1] (CSM Threats) [Agent expression][2], a CSM Threats event will be collected from the system containing all the relevant context about the activity.

This event is sent to Datadog, where it is analyzed. Based on analysis, CSM Threats events can trigger Security Signals or they can be stored as events for audit, threat investigation purposes.

CSM Threats events have the following JSON schema depending on the platform:

* [Linux][1]
* [Windows][2]

[1]: /security/threats/backend_linux
[2]: /security/threats/backend_windows