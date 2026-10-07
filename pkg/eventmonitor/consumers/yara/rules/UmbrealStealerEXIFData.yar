rule UmbrealStealerEXIFData {
    meta:
        description = "Detects UmbralStealer by obvious comment in EXIF Data"
        author = "adm1n_usa32"
        date = "2024-09-02"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_uuid = "9e454251-6212-4f36-8ebb-e64f694442e6"
        yarahub_license = "CC0 1.0"
        yarahub_reference_md5 = "83b81dda82a62350b52ee97a12d3163a"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $thishexstring = { 50 00 61 00 79 00 6C 00 6F 00 61 00 64 00 20 00 66 00 6F 00 72 00 20 00 55 00 6D 00 62 00 72 00 61 00 6C 00 20 00 53 00 74 00 65 00 61 00 6C 00 65 00 72 }
        $thistextstring = "Payload for Umbral Stealer"
    condition:
        $thishexstring or $thistextstring
}
