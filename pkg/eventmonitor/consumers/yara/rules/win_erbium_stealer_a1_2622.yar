rule win_erbium_stealer_a1_2622 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-09-01"
        description               = "detects the unpacked Erbium stealer"
        hash1_md5                 = "e719388778f14e77819a62c5759d114b"
        hash1_sha1                = "540fe15ae176cadcfa059354fcdfe59a41089450"
        hash1_sha256              = "d932a62ab0fb28e439a5a7aab8db97b286533eafccf039dd079537ac9e91f551"
        hash2_md5                 = "74f53a6ad69f61379b6ca74144b597e6"
        hash2_sha1                = "f188b5edc93ca1e250aee92db84f416b1642ec7f"
        hash2_sha256              = "d45c7e27054ba5d38a10e7e9d302e1d6ce74f17cf23085b65ccfba08e21a8d0b"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "e719388778f14e77819a62c5759d114b"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "2622fa81-d545-4b34-918c-ddc9c16d9b48"

    strings:
        $str_path            = "ErbiumDed/api.php?method=getstub&bid=" wide
        $str_tag             = "malik_here" ascii
        $fowler_noll_vo_hash = {C5 9D 1C 81 [1-100] 93 01 00 01}

    condition:
        uint16(0) == 0x5A4D and
        (
            all of ($str_*) and #fowler_noll_vo_hash >= 2
        )
}
