rule MAL_Cronos_Crypter_Salt {
    meta:
        description = "Detects Cronos Crypter based encryption salt value and string that should be seen in memory."
        author = "Tony Lambert"
        yarahub_reference_md5 = "90137ea83b86cd0f07a81156c6a633a8"
        date = "2024-03-17"
		yarahub_uuid = "c728dd41-dcfb-46c3-a60e-a34553a3dccb"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_license = "CC0 1.0"
    strings:
        $s1 = "Cronos-Crypter" ascii wide
        $salt = {1A 14 CA EA 88 7B 45 2F}
    condition:
        all of them
}