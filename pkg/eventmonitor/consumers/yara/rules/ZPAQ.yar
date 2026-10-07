rule ZPAQ {

  meta:
      description = "Detects files commpressed with ZPAQ alg."
      date = "2023-10-03"
      yarahub_reference_md5 = "72b8f5d6ed58add5bf34b7d051ce40b3"
      yarahub_uuid = "a10f3c0d-4f17-473d-8453-c82cc22e2c82"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"

  strings:
      $start_a = { 37 6b 53 74 a0 31 83 d3 8c b2 28 b0 d3 7a 50 51 02 01 07 00 00 00 00 00 00 00 00 01 6a 44 43 32 }

  condition:
      $start_a at 0
}
