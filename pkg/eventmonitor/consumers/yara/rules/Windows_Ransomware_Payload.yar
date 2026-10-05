rule Windows_Ransomware_Payload
{
    meta:
        description = "Payload ransomware"
        author = "Dark Atlas; @ELJoOker"
        date = "2026-05-21"
        yarahub_uuid = "101cf420-0e2c-4ba8-9a4d-11363317342c"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "E0FD8FF6D39E4C11BDAF860C35FD8DC0"

    strings:
        $s1 = "aH9Tbdc+qPcQkPwhclaNYFadhF04GzuGsuRxDbKMRkU=" ascii
        $s2 = "aFFFUElOdVRZMmx5dHVmTERwSlFkVmtyV2xvdkMxUVI=" ascii
        $s3 = "expand 32-byte kFBI" ascii
        $s4 = "RECOVER_payload.txt" ascii
        $s5 = ".payload" wide
        $s6 = "/c vssadmin.exe delete shadows /all /quiet" wide
        $s7 = "NtQueryDirectoryFile" ascii
        $s8 = "EtwEventWriteFull" ascii
        $s9 = "BackupExecVSSProvider" ascii

    condition:
        uint16(0) == 0x5A4D and
        uint32(uint32(0x3C)) == 0x00004550 and
        uint16(uint32(0x3C) + 4) == 0x014C and
        filesize < 1MB and
        (
            $s1 or
            $s2 or
            ($s3 and $s4 and $s5) or
            ($s6 and $s7 and $s8 and $s9)
        )
}