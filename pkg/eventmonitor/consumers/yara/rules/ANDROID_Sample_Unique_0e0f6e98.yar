rule ANDROID_Sample_Unique_0e0f6e98 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "0e0f6e98af559b306e418566132316b255e425e55b535c14bdb579edc563c640.apk"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "a6415936-d018-4a82-8caa-a3ce8200b452"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "8b1e563b7d88e1383bbce424b2fba19e"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "	u	u	N	N	#	#	<	<	N	N	:	:	|	|	L	L	Z	Z	d	d	" ascii wide
        $b = "''res/drawable-xhdpi-v4/default_image.png" ascii wide
    condition:
        all of them
}
