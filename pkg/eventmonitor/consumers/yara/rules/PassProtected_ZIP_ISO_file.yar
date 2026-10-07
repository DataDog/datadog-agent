rule PassProtected_ZIP_ISO_file {
   meta:
      description = "Detects container formats commonly smuggled through password-protected zips"
      author = "_jc"
      date = "2022-09-29"
      yarahub_reference_md5 = "b93bd94b8f568deac0143bf93f7d8bd8"
      yarahub_uuid = "0b027752-0217-48f9-9515-3760872cc210"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
   strings:
      $password_protected_zip = { 50 4B 03 04 14 00 01 }

      $container_1 = ".iso" ascii
      $container_2 = ".rr0" ascii
      $container_3 = ".img" ascii
      $container_4 = ".vhd" ascii
      $container_5 = ".rar" ascii

   condition:
      uint32(0) == 0x04034B50 and
      filesize < 2000KB and 
      $password_protected_zip and 
      1 of ($container*)
}