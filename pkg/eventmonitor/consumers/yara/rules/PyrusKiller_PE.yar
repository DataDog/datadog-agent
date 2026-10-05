rule PyrusKiller_PE
{
    meta:
        id = "owlyO1OFyQfF3bstJOcyE5"
        fingerprint = "709450409f476f8de45ea53fb47451c3d4bb24daabeee5eae8b52530aa483ba8"
        version = "1.1"
        date = "2026-03-10"
        modified = "2026-03-10"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "PyrusKiller ransomware - PyInstaller packed PE with encrypted payload"
        category = "MALWARE"
        malware = "PYRUSKILLER"
        mitre_att = "T1486"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "PyrusKiller ransomware PyInstaller packed PE detected."
        yarahub_uuid = "bfbbaaa9-40d2-4904-9291-1ee0be68bb25"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e16050e5a651252679b4ba831d7551b6"

    strings:
        // PyInstaller archive entry for the ransomware entry point
        // TOC entry: typecode 's' (SCRIPT) followed by null-terminated name
        $toc_entry = "sPyrusKiller" ascii

        // PyInstaller bootloader markers
        $pyiboot = "pyiboot01_bootstrap" ascii
        $meipass = "_MEIPASS" ascii

        // Python 3.10 runtime DLL name in the archive TOC
        $py310 = "python310.dll" ascii

    condition:
        uint16(0) == 0x5A4D
        and filesize > 10MB and filesize < 50MB
        and $toc_entry
        and ($pyiboot or $meipass)
        and $py310
}
