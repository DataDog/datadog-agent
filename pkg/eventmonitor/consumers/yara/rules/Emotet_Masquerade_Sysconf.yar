rule Emotet_Masquerade_Sysconf {
    meta:
        author = "tranquil"
        date = "2026-06-07"
        description = "Detects Emotet variant masquerading as legitimate Windows Management Service (sysconf.exe)"
        reference = "Internal Lab Analysis"
        yarahub_uuid = "6cc502fe-84b3-42e7-ae77-fb5598518f5b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "803385cf25070740f5b09e685d2f531c"

    strings:
        $fake_desc = "Windows Management Service" wide nocase
        $fake_file = "sysconf.exe" wide nocase
        $lang_id   = "040904B0" wide
        $fake_comp = "CompanyName" wide

    condition:
        uint16(0) == 0x5A4D and 
        filesize < 15MB and
        all of them
}
