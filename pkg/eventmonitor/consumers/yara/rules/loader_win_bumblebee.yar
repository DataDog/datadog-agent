rule loader_win_bumblebee {
   meta:
      author = "SEKOIA.IO"
      description = "Find BumbleBee samples based on specific strings"
      date = "2022-06-02"
      yarahub_author_twitter = "@sekoia_io"
      yarahub_reference_link = "https://blog.sekoia.io/bumblebee-a-new-trendy-loader-for-initial-access-brokers/"
      yarahub_reference_md5 = "6d58437232ebab24d810270096e6e20b"
      yarahub_uuid = "8fd795c7-6896-498c-a892-de9da6427b60"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.bumblebee"

   strings:
      $str0 = { 5a 00 3a 00 5c 00 68 00 6f 00 6f 00 6b 00 65 00 72 00 32 00 5c 00 43 00 6f 00 6d 00 6d 00 6f 00 6e 00 5c 00 6d 00 64 00 35 00 2e 00 63 00 70 00 70 00 } // Z:\hooker2\Common\md5.cpp
      $str1 = "/gates" ascii
      $str2 = "3C29FEA2-6FE8-4BF9-B98A-0E3442115F67" wide

   condition:
      uint16be(0) == 0x4d5a and all of them
}
