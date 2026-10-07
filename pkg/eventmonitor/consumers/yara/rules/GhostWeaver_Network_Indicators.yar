rule GhostWeaver_Network_Indicators
{
    meta:
        id = "5nlhxIWY2MajrhjApbjA04"
        fingerprint = "157dc22665c263d58479b9faf6999303a3c766faf2733cfd927e542a81a88061"
        version = "1.0"
        date = "2026-03-08"
        modified = "2026-03-08"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://WWW.DERP.CA/BLOG/GHOSTWEAVER-TAG124-POWERSHELL-RAT"
        author = "derp.ca"
        description = "GhostWeaver/Pantera network-level indicators (C2 traffic, DGA domains)"
        category = "MALWARE"
        malware = "GHOSTWEAVER"
        mitre_att = "T1568.002"
        reference = "https://www.derp.ca/blog/ghostweaver-tag124-powershell-rat"
        triage_score = 10
        triage_description = "GhostWeaver/Pantera C2 beacon or command traffic detected."
        yarahub_uuid = "b86558f7-e506-49f8-89c6-acc2480f187b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "78113996c94c5a1fcacba8da4ca86dde"

    strings:
        $json_clientinfo = /\"Packet\":\s?\"ClientInfo\"/ ascii nocase
        $json_ping = /\"Packet\":\s?\"Ping\"/ ascii nocase
        $json_pong = /\"Packet\":\s?\"pong\"/ ascii nocase
        $json_hwid = /\"HWID\":\s?\"/ ascii
        $json_version = /\"Version\":\s?\"/ ascii
        $json_group = /\"Group\":\s?\"/ ascii
        $json_plugin = /\"Packet\":\s?\"plugin\"/ ascii nocase
        $json_sendplugin = /\"Packet\":\s?\"sendPlugin\"/ ascii nocase
        $json_iex = /\"Packet\":\s?\"iex\"/ ascii nocase

    condition:
        ($json_clientinfo and $json_hwid and $json_version and $json_group)
        or
        ($json_plugin and $json_sendplugin)
        or
        (($json_pong or $json_ping) and ($json_iex or $json_plugin))
}
