rule ZeroDay_Campaign_Config : config malware
{
    meta:
        author = "CyberStrike"
        description = "ZeroDay RAT campaign configuration file with admin token and payment config"
        date = "2026-09-05"
        yarahub_author_twitter = "@XenoSheesh"
        yarahub_reference_md5 = "35b2d781134a7bf705a357ec3b314910"
        yarahub_uuid = "26457f42-6fbf-43c3-ae61-095d3670c331"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        
    strings:
        $key1 = "admin_token" ascii wide
        $key2 = "app_type" ascii wide
        $key3 = "user_id" ascii wide
        
        $pay1 = "Guest Registration Fee" ascii wide
        $pay2 = "currency_symbol" ascii wide
        
        $flavor1 = "weddingv2" ascii wide nocase
        $flavor2 = "sexychat" ascii wide nocase
        $flavor3 = "mparivahan" ascii wide nocase
        
    condition:
        filesize < 10KB and
        (
            (all of ($key*)) or
            (any of ($pay*) and any of ($key*)) or
            (any of ($flavor*) and $key1)
        )
}
