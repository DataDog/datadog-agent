rule shubstealer_loader_zsh {
    meta:
        author      = "Lenny-3BO"
        description = "Detects SHubStealer stage-1 zsh loader by shub_* markers and CIS geo-filter pattern"
        family      = "osx.shubstealer"
        tlp         = "WHITE"
        date        = "2026-04-17"
        hash        = "bb38b1a974547f77b8c59b7efdb5add8"
        yarahub_reference_md5 = "bb38b1a974547f77b8c59b7efdb5add8"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_uuid = "f425c2dc-6e89-4bbc-8814-7c21a0444466"

    strings:
        $shub_split  = "shub_split"  ascii
        $shub_mzip   = "shub_mzip"   ascii
        $shub_prefix = /shub_[a-z]{3,12}/ ascii
        $cis_check   = "HIToolbox" ascii
        $cis_layout  = "KeyboardsLayout" ascii
        $pewwee      = "pewweepor092" ascii

    condition:
        filesize < 500KB and
        (
            ($shub_split or $shub_mzip or $shub_prefix) and
            ($cis_check or $cis_layout) and
            $pewwee
        )
}
