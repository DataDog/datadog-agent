rule PhantomStealerV4
{
    meta:
        family = "PhantomStealerV4"
        decoder = "PhantomStealerV4"
        date = "2026-09-26"
        author = "nsquar3"
        description = "Detects PhantomStealerV4 samples"
        yarahub_uuid = "23255f48-dfd8-47a4-bec3-8ccf5a0c66ba"
        yarahub_reference_md5 = "1b49e76b535b70e274b5240b9417f123"
        yarahub_version = "2"
        yarahub_tags = "phantom,stealer,trojan,adk"
        yarahub_threat_type = "malware"
        yarahub_threat_name = "PhantomStealerV4"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_threat_category = "info-stealer"
        yarahub_threat_family = "PhantomStealer"
        yarahub_license = "CC0 1.0"
        yarahub_confidence = "High"
        yarahub_tlp = "WHITE"
        
    strings:
        $a = "PhantomStealer4"
    condition:
        uint16(0) == 0x5A4D and $a
}
