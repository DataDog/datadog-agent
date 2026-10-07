rule ZeroDay_C2_Domain : c2 malware network
{
    meta:
        author = "CyberStrike"
        description = "ZeroDay RAT C2 domain agenticera.club - panel and API infrastructure"
        date = "2026-09-05"
        yarahub_author_twitter = "@XenoSheesh"
        yarahub_reference_md5 = "35b2d781134a7bf705a357ec3b314910"
        yarahub_uuid = "5bf09945-9b17-4b16-aa04-d6c0ba2b2605"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        
    strings:
        $domain1 = "agenticera.club" ascii wide nocase
        $domain2 = "api.agenticera.club" ascii wide nocase
        $domain3 = "panel.agenticera.club" ascii wide nocase
        
        $ws1 = "wss://api.agenticera.club" ascii wide
        $ws2 = "/cmd/device" ascii wide
        $ws3 = "/ws/tunnel" ascii wide
        
    condition:
        any of ($domain*) or
        ($ws1 and any of ($ws2, $ws3))
}
