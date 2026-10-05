rule mofongo_loader
{
    meta:
        author = "vrzh"
        description = "Mofongo loader maps and executes a payload in a hollowed msedge process"
        date = "2024-05-03"
        hash_md5 = "d74cf6a901a529abb68a9c0fbbc1034b"
        yarahub_uuid = "0d461828-f851-46ba-826b-4fbfa7e30461"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d74cf6a901a529abb68a9c0fbbc1034b"
        yarahub_reference_link= "https://malware.boutique/posts/mofongo"
        yarahub_author_twitter = "@_vrzh"


    strings:
        // A peculiar string decryption routine; serves as a good signature.
        $string_decryption_0 = {
            b9 ?? 00 00 00 f7 f9 8b c2 83 c0 ?? 8b 4c 24 ?? 33 c8 8b c1 48 63
            0c 24 48 8b 54 24 ?? 88 04 0a
        }
    condition:
        uint16(0) == 0x5A4D and $string_decryption_0
}
