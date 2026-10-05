rule anyburn_iso_with_date {
    meta:
        author = "Nils Kuhnert"
        date = "2022-12-22"
        description = "Triggers on ISOs created with AnyBurn using volume names such as 12_19_2022."
        hash1_md5 = "e01931b3aba4437a92578dc802e5c41d"
        hash1_sha1 = "00799e6150e97f696635718d61f1a4f993994b87"
        hash1_sha256 = "87d51bb9692823d8176ad97f0e86c1e79d704509b5ce92b23daee7dfb2d96aaa"
        yarahub_reference_md5 = "e01931b3aba4437a92578dc802e5c41d"
        yarahub_author_twitter = "@0x3c7"
        yarahub_uuid = "0f217560-0380-458a-ac9a-d9d3065e22d9"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $volume_name = { 43 44 30 30 31 01 00 00 57 00 69 00 6e 00 33 
                         00 32 00 20 00 20 00 20 00 20 00 20 00 20 00 20 
                         00 20 00 20 00 20 00 20 00 3? 00 3? 00 5f 00 3?
                         00 3? 00 5f 00 3? 00 3? 00 3? 00 3? 00 20 00 20 }
        $anyburn = "AnyBurn" wide fullword
    condition:
        all of them
}
