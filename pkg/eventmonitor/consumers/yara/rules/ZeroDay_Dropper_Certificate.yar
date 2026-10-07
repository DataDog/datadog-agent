rule ZeroDay_Dropper_Certificate : certificate malware
{
    meta:
        author = "CyberStrike"
        description = "ZeroDay dropper APK signing cert - NovaCraft Labs / UrbanCraft Apps (India)"
        date = "2026-09-05"
        yarahub_author_twitter = "@XenoSheesh"
        yarahub_reference_md5 = "35b2d781134a7bf705a357ec3b314910"
        yarahub_uuid = "3718d721-b1c9-4993-902b-4d8124198aec"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        cert_sha256 = "7285d838ab5ab387388a6ebaae202d34f82eb85d620b76ba69a1eb7266b662fe"
        
    strings:
        $org = "NovaCraft Labs" ascii wide
        $cn = "UrbanCraft Apps" ascii wide
        $loc = "Gurugram" ascii wide
        
    condition:
        uint32(0) == 0x04034b50 and
        (all of them)
}
