rule ZeroDay_Payload_Certificate : certificate malware
{
    meta:
        author = "CyberStrike"
        description = "ZeroDay payload APK signing cert - TrueNorth Apps / Food Recipes (India)"
        date = "2026-09-05"
        yarahub_author_twitter = "@XenoSheesh"
        yarahub_reference_md5 = "3c53caa2fd562d12dcf22bd5b396a2ed"
        yarahub_uuid = "cc3bb055-0830-4dc5-8067-2e61021ab4a3"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        cert_sha256 = "458d59e0cf0406baaf7562e792b86d5605c835a117f425a9f1fde599aa61740e"
        
    strings:
        $org = "TrueNorth Apps" ascii wide
        $cn = "Food Recipes" ascii wide
        
    condition:
        uint32(0) == 0x04034b50 and
        (all of them)
}
