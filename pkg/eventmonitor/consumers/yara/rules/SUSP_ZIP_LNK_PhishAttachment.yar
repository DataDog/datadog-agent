rule SUSP_ZIP_LNK_PhishAttachment {
    meta:
        description = "Detects suspicius tiny ZIP files with malicious lnk files"
        author = "ignacior"
        reference = "Internal Research"
        date = "2022-06-23"
        score = 50
        yarahub_uuid = "fbb7c8e8-55b6-4192-877b-3dbaad76e12e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a457d941f930f29840dc8219796e35bd"
    strings:
        $sl1 = ".lnk"
    condition:
		uint16(0) == 0x4b50 and filesize < 2KB and $sl1 in (filesize-256..filesize)
}
