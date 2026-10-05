rule FakeInstaller_KeyFile_Crepectl
{
    meta:
        description = "Detects the binary .key config used by the analyzed fake installer family"
        author = "SixHands"
        date = "2026-04-21"
        yarahub_uuid = "85813b75-f2c3-4032-8a79-48d3e205b492"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "a6c4948ec670f6562d7007c3a6f1ec2b"

    strings:
        $key_struct = { 01 0D 00 0B 00 0C 00 72 65 73 6F 75 72 63 65 73 2E 70 61 6B 54 71 44 4C 6D 72 42 62 6D 79 77 63 72 65 70 65 63 74 6C 2E 65 78 65 }

    condition:
        $key_struct
}