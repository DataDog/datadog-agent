rule Detect_Dead_Family
{
    meta:
        description = "YARA rule for detecting files related to dead.dll family"
        author = "Your Name"
        date = "2025-01-14"
        family = "dead.dll"
        yarahub_uuid = "65069d71-1f5a-4394-bd79-0067ae7b60a4"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "5df15af3cff38a908d5807f8ee5f8055"

    strings:
        // Common strings from metadata or the file
        $filename = "dead.dll" ascii wide
        $productname = "dead" ascii wide
        $companyname = "dead" ascii wide
        $fileversion = "1.0.0.0" ascii wide
        
        // Common patterns in entry point or code
        $entry_point_pattern = { 48 89 4C 24 ?? 48 89 54 24 ?? 4C 89 44 24 ?? }

        // Section names
        $section_BOOT = "BOOT" ascii
        $section_INIT = "INIT" ascii
        $section_KERNEL = "KERNEL" ascii

    condition:
        uint16(0) == 0x5A4D and      // PE file signature
        filesize < 600000 and        // Allow similar file sizes, +/- range
        3 of ($filename, $productname, $companyname, $fileversion) and
        $entry_point_pattern and
        2 of ($section_BOOT, $section_INIT, $section_KERNEL)
}
