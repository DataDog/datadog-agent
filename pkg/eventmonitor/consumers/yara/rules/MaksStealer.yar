rule MaksStealer {
  meta:
    author = "ShadowOpCode"
    description = "Detects MaksStealer main payload"
    last_modified = "2025-05-18"
    date = "2025-08-19"
    yarahub_uuid = "686f9629-f84e-4cff-aebc-3a2a2d9e075d"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "00000000000000000000000000000000"

  strings:
    $sig = "HellomynameisMaxIm17IlovemakingRAT" ascii
    $sig2 = "Max/Maxt" ascii

  condition:
    $sig or $sig2
}