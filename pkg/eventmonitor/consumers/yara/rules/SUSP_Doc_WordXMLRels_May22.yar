rule SUSP_Doc_WordXMLRels_May22 {
   meta:
      description = "Detects a suspicious pattern in docx document.xml.rels file as seen in CVE-2022-30190 / Follina exploitation"
      author = "Tobias Michalski, Christian Burkard, Wojciech Cieslak"
      date = "2022-05-30"
      yarahub_reference_md5 = "5f15a9b76ad6ba5229cb427ad7c7a4f6"
      yarahub_uuid = "a9aad367-682e-440c-8732-dc414274b5c3"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
	  techniques = "File and Directory"
      modified = "2022-06-02"
      reference = "https://doublepulsar.com/follina-a-microsoft-office-code-execution-vulnerability-1a47fce5629e"
      hash = "62f262d180a5a48f89be19369a8425bec596bc6a02ed23100424930791ae3df0"
      score = 70
   strings:
      $a1 = "<Relationships" ascii
      $a2 = "TargetMode=\"External\"" ascii

      $x1 = ".html!" ascii
      $x2 = ".htm!" ascii
   condition:
      filesize < 50KB
      and all of ($a*)
      and 1 of ($x*)
}

