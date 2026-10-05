rule ClipperDLL_Amadey {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-10-19"
        description = "Detects Amadey's Clipper DLL"
        yarahub_uuid = "6185c299-b3fe-4a8a-99f8-be4128566163"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "fc4faaa0d820e943dbf1235a84ae165e"
        malpedia_family = "win.amadey"

    strings:
        $ClipperDLL = "??4CClipperDLL@" ascii
        $CLIPPERDLL_dll = "CLIPPERDLL.dll" ascii

    condition:
        uint16(0) == 0x5a4d and
	any of them
}