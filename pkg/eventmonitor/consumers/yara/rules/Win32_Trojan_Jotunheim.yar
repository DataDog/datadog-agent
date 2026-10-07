rule Win32_Trojan_Jotunheim {
    meta:
        description = "Detects Jotunheim malware samples based on C2 domain indicators."
        author = "Serhii Kocherhan"
        date = "2026-09-19"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "70339763-08b9-4ebc-88f5-272ae1705298"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "61017386c63683345b0abfad7f12c2aa"

    strings:
        $domain1 = "jotunheim.name" ascii wide nocase
        $domain2 = "vanaheim.cn" ascii wide nocase

    condition:
        any of ($domain*)
}