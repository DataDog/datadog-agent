rule Babuk_Payload_Ransomware_Broad
{
    meta:
        id = "689B5BmcDeNHVlWHg9Un1X"
        fingerprint = "29cadc3230cb2d2b2632ffc4d8d1583bff07c7d7b64c4db124c35df4a315cce7"
        version = "1.0"
        date = "2026-03-15"
        modified = "2026-03-15"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "Babuk Payload ransomware - broader detection for variant builds with different operator keys or configs"
        category = "MALWARE"
        malware = "BABUK"
        malware_type = "RANSOMWARE"
        mitre_att = "T1486"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "Babuk Payload ransomware variant detected. ChaCha20 file encryption with FBI footer key."
        yarahub_uuid = "c7df3198-3a04-471c-945c-25d05d6d302b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e0fd8ff6d39e4c11bdaf860c35fd8dc0"

    strings:
        // ChaCha20 constant + FBI footer key (contiguous .rdata layout specific to Payload variant)
        $chacha_fbi = { 65 78 70 61 6E 64 20 33 32 2D 62 79 74 65 20 6B 46 42 49 00 }

        // Ransom note filename
        $note = "RECOVER_payload.txt" wide

        // Extension
        $ext = ".payload" wide

        // Log prefixes unique to the Payload builder
        $log_enc = "[Encryption] " wide
        $log_args = "[Args] " wide
        $log_disk = "[Disk] " wide

        // Mutex name
        $mutex = "MakeAmericaGreatAgain" wide

        // Pubkey size validation
        $pubkey_check = "SIZE OF PUBKEY IS LOWER THAN NEED" wide

        // Process kill list (common Babuk set, at least 3 in sequence)
        $proc1 = "sql.exe" wide
        $proc2 = "oracle.exe" wide
        $proc3 = "ocssd.exe" wide
        $proc4 = "dbsnmp.exe" wide
        $proc5 = "sqbcoreservice.exe" wide

        // Service kill (unique combo)
        $svc1 = "BackupExecVSSProvider" ascii
        $svc2 = "VeeamDeploymentService" ascii
        $svc3 = "zhudongfangyu" ascii

    condition:
        uint16(0) == 0x5A4D
        and filesize > 100KB and filesize < 2MB
        and (
            // Core: ChaCha+FBI key pattern
            ($chacha_fbi and ($note or $ext or $mutex))
            or
            // Note + extension + log framework
            ($note and $ext and 2 of ($log_enc, $log_args, $log_disk))
            or
            // Process kill list + extension + pubkey validation
            (3 of ($proc1, $proc2, $proc3, $proc4, $proc5) and $ext and $pubkey_check)
            or
            // Service kill combo + mutex + note
            (2 of ($svc1, $svc2, $svc3) and $mutex and $note)
        )
}
