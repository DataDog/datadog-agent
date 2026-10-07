rule Ransom_newRaaS : Ransomware
{
    meta:
        name                        = "Ransom_newRaaS"
        category                    = "Ransomware"
        description                 = "NewbirthRaas_ransomware"
        author                      = "Valton Tahiri"
        created                     = "2025-10-21"
        date                        = "2025-10-21"
        tlp                         = "TLP:white"
        reliability                 = 75
        sample                      = "a0d98d2b8035b3ecc4d0c51736c73e7a62ae95d5f31ca7b21c128d44256cb7b4"
        yarahub_uuid                = "14b84c9c-12f9-40e5-b4eb-9fc33b6c35b1"
        yarahub_license             = "CC0 1.0"
        yarahub_rule_matching_tlp  = "TLP:WHITE"
        yarahub_rule_sharing_tlp   = "TLP:WHITE"
        yarahub_reference_md5      = "c928aed48047cec64495d7d1daf21dc2"

    strings:
        $s1 = "cmd.exe /c C:\\Windows\\System32\\wbem\\WMIC.exe shadowcopy where \"ID='%s'\" delete" wide
        $s2 = "-net requires at least -nethost, -netuser, -netpass, -netdomain" wide
        $s3 = "Processes to kill:\n" wide

    condition:
        2 of them
}
