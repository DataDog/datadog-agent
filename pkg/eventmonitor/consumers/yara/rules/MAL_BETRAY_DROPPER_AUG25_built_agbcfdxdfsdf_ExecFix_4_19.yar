/*
   YARA Rule
   Author: DeTraced Security
   Date: 2025-10-17
   Identifier: MAL_BETRAY_DROPPER_AUG25
   Reference: https://detraced.org/posts/infostealer-turned-ransomware
*/

rule MAL_BETRAY_DROPPER_AUG25_built_agbcfdxdfsdf_ExecFix_4_19 {
   meta:
      description = "MAL_BETRAY_DROPPER_AUG25 - from files built-agbcfdxdfsdf.bat, ExecFix-4.bat"
      author = "DeTraced Security"
      reference = "https://detraced.org/posts/infostealer-turned-ransomware"
      date = "2025-10-17"
      yarahub_author_twitter = "@DeTraced-Sec"
      yarahub_author_email = "detraced-sec@proton.me"
      yarahub_uuid = "3aaba4a2-d618-4247-8169-b88d3e417afa"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      yarahub_reference_md5 = "780c2f5e127d12181b6650cededecd58"
      hash1 = "75b768ef203659bcc8134b36689732adbd2a50fa5a522270042026a17a1a2bfb"
      hash2 = "7c51cb0ae8f3e5c72bb09c67aafd5d68dc9cc01d222ba1fc2f1d3eb8672ab287"
   strings:
      $s1 = "BIiWwkGEiJdCQgV0FWQVdIg+wwM/Yz7UUz9kiL+kyL+UKKTD0AhMl0FIP9QHQPiEw0IP/F/8aD/hB1Z+tTi8ZIjVwkIEgD2EG4EAAAAEiLy0QrxjPS6M0EAADGA4CD/g" ascii
      $s2 = "RIvYQYP6G3LNiUwkKESJRCQsSItEJChIg8QQW8NFhckPhEUBAABIiVwkCEiJdCQQSIl8JBhVQVRBVkiL7EiD7BBMi9lIjUXwTCvYTI1yD0mL+EG8EAAAAEiNRfBJO8Z3" ascii
      $s3 = "LSIlF4CPOg8ECQdHrdCFIi1XoRYvDSffYQYoEEIgCSAPWQQPOdfJIiVXo6fwAAACL/un1AAAARIvWSI1N4OgyAQAASI1N4EaNFFDoJQEAAIXAdeZFhcl1SEGD+gJ1QkS" ascii
      $s4 = "sAwgQAuAFAAIDCCABWV+g+JQAAi3QkELmtEUAAv3s2QAArzwPBiw6JAegjJQAAuR8RQAArzwPBiw6JQQToECUAALkMEkAAK88DwYsOiUEI6P0kAAC5LhFAACvPA8GLDo" ascii
      $s5 = "lBDOjqJAAAuS4RQAArzwPBiw5fiUEQi0QkCINmBACJRghew4tMJAyFyXUHuANAAIDrTVOLXCQMM9JWi3QkDFeLfgiLhJcwCAAAOwSTdQhCg/oEde7rFDPSi4SX8AgAAD" ascii
   condition:
      ( uint16(0) == 0x6540 and filesize < 8000KB and ( all of them )
      ) or ( all of them )
}