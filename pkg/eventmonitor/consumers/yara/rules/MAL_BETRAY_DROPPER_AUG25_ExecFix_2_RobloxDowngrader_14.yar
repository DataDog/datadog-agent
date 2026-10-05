/*
   YARA Rule
   Author: DeTraced Security
   Date: 2025-10-17
   Identifier: MAL_BETRAY_DROPPER_AUG25
   Reference: https://detraced.org/posts/infostealer-turned-ransomware
*/

rule MAL_BETRAY_DROPPER_AUG25_ExecFix_2_RobloxDowngrader_14 {
   meta:
      description = "MAL_BETRAY_DROPPER_AUG25 - from files ExecFix-2.bat, RobloxDowngrader.bat"
      author = "DeTraced Security"
      reference = "https://detraced.org/posts/infostealer-turned-ransomware"
      date = "2025-10-17"
      yarahub_author_twitter = "@DeTraced-Sec"
      yarahub_author_email = "detraced-sec@proton.me"
      yarahub_uuid = "3d9714d4-73b6-40c0-b62d-a9edd4269387"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      yarahub_reference_md5 = "51b03d9bae4a53dd4e6210254084b806"
      hash1 = "c9f3f1d7e7d8b6cb5630018c3bdc9ec9c5d069b47aabe3ddda630c93b2e0ede4"
      hash2 = "aa8af03833ca5030045f2937ccd1361217fa8515e85bfb63dbcfcc5162273a84"
   strings:
      $s1 = "%AoziAsoApppsdAJzhGAahjXHHagHGSHFahJAHjshjHJhajHAJhsjhJAHjahvxgvGAVGcdAGgavgsvdgAVG% \"shAxhAGDJahADA%XzAdxACashaAUXGaidhAGDYAYX" ascii
      $s2 = "ayYSuyxUAXusgzgagsfxYAGdsgAHXDFAhadfAHXGa%exit /b \" " fullword ascii
      $s3 = "UkEQf/USIvwSIXAD4T0AAAAQbgwBQAASIvTSIvI6KIdAACLQwiNSP1BO852GoP4AnVdSI2WKAUAAEiNiygFAADoAxsAAOtFRIuDJAUAAEyNiygFAAAPt8hIjZYoBQAAZ" ascii
      $s4 = "IXJdCVEi4dYDQAAM9LoshwAAEiLj2ANAABEi8Yz0kH/10iDp2ANAAAARIsHM9KLnzACAABIi8/oiBwAAESLxjPSSIvPQf/Xg/sCdQQzyf/VM8Dpsvz//8xIiVwkCFVWV" ascii
      $s5 = "3AFAAB0MkiLz+hW7f//hcB1DYO/cAUAAAIPhEYBAABIi8/oae7//4XAdQ2Dv3AFAAACD4QtAQAARDlzCA+ErQAAAIuTJAUAADPJSIHCLxUAAEG4ADAAAEiB4gDw//9Ej" ascii
      $s6 = "kErzrgAAQAAZgvISI2EJLABAABIiUQkKIuDIAUAAIlEJCD/lwACAACFwHV5SIveiwuNQf1BO8Z2UI1B/0E7xnYVjUH7QTvGd0tIi9NIi8/oqA8AAOs+TI1EJDBIi9NIi" ascii
      $s7 = "8/oYvr//4XAdBBMjUQkMEiL00iLz+ieAAAASI1UJDBIi8/o0fT//+sLSIvTSIvP6LQEAACDvzACAAADdQjr/kG+AQAAAIuHIAkAAL4AwAAAg+gCQTvGdzFIi49gDQAAS" ascii
   condition:
      ( uint16(0) == 0x6540 and filesize < 8000KB and ( all of them )
      ) or ( all of them )
}