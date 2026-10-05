rule blocksig : branchlock
{
    meta:
        date = "2024-08-08"
        yarahub_reference_md5 = "37d9c9c214040d54e8d7219b851ca3f2"
        yarahub_uuid = "f38eaaf6-f1eb-44ef-a93f-8bdc74ec5b58"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $blocksig = { 4F 62 66 75 73 63 61 74 65 64 20 75 73 69 6E 67 20 74 68 65 20 42 72 61 6E 63 68 6C 6F 63 6B 20 6F 62 66 75 73 63 61 74 6F 72 20 66 6F 72 20 6A 61 76 61 20 2D 20 68 74 74 70 73 3A 2F 2F 62 72 61 6E 63 68 6C 6F 63 6B 2E 6E 65 74 }
    condition:
    $blocksig
}
