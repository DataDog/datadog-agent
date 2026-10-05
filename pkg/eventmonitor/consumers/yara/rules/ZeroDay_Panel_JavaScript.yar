rule ZeroDay_Panel_JavaScript : webpanel c2 malware
{
    meta:
        author = "CyberStrike"
        description = "ZeroDay Control Panel - Flutter web JavaScript bundle for RAT management"
        date = "2026-09-05"
        yarahub_author_twitter = "@XenoSheesh"
        yarahub_reference_md5 = "35b2d781134a7bf705a357ec3b314910"
        yarahub_uuid = "9be319d6-999d-4117-ab64-c9720b9b6317"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        
    strings:
        $brand1 = "ZeroDay Control Panel" ascii wide
        $brand2 = "ZeroDay" ascii wide
        
        $api1 = "api.agenticera.club" ascii wide
        $api2 = "panel.agenticera.club" ascii wide
        $api3 = "/auth/login" ascii wide
        $api4 = "/api/devices" ascii wide
        
        $storage1 = "zd_access_token" ascii wide
        $storage2 = "zd_admin_info" ascii wide
        
        $role1 = "supreme" ascii wide
        $role2 = "super_admin" ascii wide
        
        $cmd1 = "upload_all_sms" ascii wide
        $cmd2 = "upload_all_contacts" ascii wide
        $cmd3 = "call_forwarding" ascii wide
        
        $supreme1 = "/api/supreme/intercept" ascii wide
        $supreme2 = "intercept-config" ascii wide
        
        $flutter = "dartProgram" ascii
        
    condition:
        filesize > 100KB and
        filesize < 50MB and
        (
            (any of ($brand*) and 2 of ($api*)) or
            (all of ($storage*) and any of ($role*)) or
            (2 of ($cmd*) and any of ($api*)) or
            (all of ($supreme*)) or
            ($flutter and $api1)
        )
}
