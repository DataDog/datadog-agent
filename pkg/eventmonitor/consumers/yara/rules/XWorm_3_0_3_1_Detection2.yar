rule XWorm_3_0_3_1_Detection2 {
    meta:
	yarahub_uuid = "687740d6-e1b9-4284-878b-93a888db382d"
	yarahub_license = "CC0 1.0"
	yarahub_rule_matching_tlp = "TLP:WHITE"
	yarahub_rule_sharing_tlp = "TLP:WHITE"
	yarahub_reference_md5 = "1b80b6637f49a08fbedbed6f7a80584f"
        author = "Archevod11"
        description = "Detects XWorm versions 3.0 and 3.1 - New"
        version = "1.0"
        date = "2024-06-17"
        malware_family = "XWorm"

    strings:
        // Strings unique to XWorm 3.0 and 3.1
        $version_3_0 = "XWorm 3.0" wide ascii
        $version_3_1 = "XWorm 3.1" wide ascii

    condition:
        // Match if any version-specific strings are found
        any of ($version_3_0, $version_3_1)
}
