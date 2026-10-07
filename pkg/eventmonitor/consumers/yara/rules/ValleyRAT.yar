rule ValleyRAT {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2025-01-03"
        description = "Detects ValleyRAT"
        yarahub_uuid = "321c7e27-a4ba-4d1a-8a25-4d8dbb5e6a8f"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "2a0ab82d1a8a147acf8299116c34378c"

    strings:
        $InjectionTarget = {2E 65 78 65 [2-10] 25 73 25 73}
        $WriteMiniDump = {4D 69 6E 69 44 75 6D 70 57 72 69 74 65 44 75 6D 70 [2-10] 21 00 61 00 6E 00 61 00 6C 00 79 00 7A 00 65 00 20 00 2D 00 76}
        $dmpFile = "%s-%04d%02d%02d-%02d%02d%02d.dmp" fullword wide ascii

    condition:
        2 of them
}