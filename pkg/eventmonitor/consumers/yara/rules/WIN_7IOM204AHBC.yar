rule WIN_7IOM204AHBC
{
    meta:
        author = "Netresec"
        description = "Detects files and emails containing the windows hostname WIN-7IOM204AHBC."
        date = "2026-03-22"
        reference = "https://infosec.exchange/@netresec/116262499588275546"
        yarahub_reference_md5 = "708a9ada0f7c3d655d8356a3f137be31"
        yarahub_uuid = "04213bf7-3ced-428b-b94c-82ecace7d0b3"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_author_twitter = "@netresec"
        yarahub_reference_link = "https://infosec.exchange/@netresec/116262499588275546"
    strings:
        $hostname = "WIN-7IOM204AHBC"

    condition:
        $hostname
}