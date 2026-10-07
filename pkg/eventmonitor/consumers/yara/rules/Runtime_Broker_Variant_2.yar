rule Runtime_Broker_Variant_2 {
   meta:
      description = "Detecting malicious Runtime Broker"
      author = "Sn0wFr0$t"
      date = "2025-06-01"
      yarahub_uuid = "e820a014-bf4b-40ed-b9ce-2d7f5d3571f0"
	  yarahub_license = "CC0 1.0"
	  yarahub_rule_matching_tlp = "TLP:WHITE"
	  yarahub_rule_sharing_tlp = "TLP:WHITE"
	  yarahub_reference_md5 = "9245cdd50168dcf0115ab60324114c07"
   strings:
      $x1 = "C:\\Users\\user\\Desktop\\DotNetTor\\src\\DotNetTor\\obj\\Release\\netstandard2.0\\DotNetTor.pdb" fullword ascii
      $s2 = "ESystem.Net.Http.HttpMessageHelper+<GetDecodedChunkedContentAsync>d__5" fullword ascii
      $s3 = "ESystem.Net.Http.HttpMessageHelper+<GetDecodedChunkedContentAsync>d__7" fullword ascii
      $s4 = "ESystem.Net.Http.HttpMessageHelper+<GetDecodedChunkedContentAsync>d__6" fullword ascii
      $s5 = "BSystem.Net.Http.HttpMessageHelper+<GetContentTillLengthAsync>d__11" fullword ascii
      $s6 = "7System.Net.Http.HttpMessageHelper+<GetContentAsync>d__3" fullword ascii
      $s7 = "7System.Net.Http.HttpMessageHelper+<GetContentAsync>d__4" fullword ascii 
      $s8 = "DotNetTor.dll" fullword wide
      $s9 = "?System.Net.Http.HttpMessageHelper+<GetContentTillEndAsync>d__10" fullword ascii
      $s10 = "8System.Net.Http.HttpMessageHelper+<ReadHeadersAsync>d__1" fullword ascii 
      $s11 = "Failed to send command to TOR Control Port: {0} : {1}" fullword wide 
      $s12 = "4DotNetTor.ControlPort.Client+<SendCommandAsync>d__15" fullword ascii 
      $s13 = "4DotNetTor.ControlPort.Client+<SendCommandAsync>d__16" fullword ascii 
      $s14 = "HttpResponseContentHeaders" fullword ascii 
      $s15 = "HttpRequestContentHeaders" fullword ascii 
      $s16 = "<GetDecodedChunkedContentAsync>d__7" fullword ascii 
      $s17 = "ASystem.Net.Http.HttpResponseMessageExtensions+<ToStreamAsync>d__1" fullword ascii 
      $s18 = "ASystem.Net.Http.HttpMessageHelper+<ReadBytesTillLengthAsync>d__12" fullword ascii 
      $s19 = "BSystem.Net.Http.HttpResponseMessageExtensions+<CreateNewAsync>d__0" fullword ascii
      $s20 = "ASystem.Net.Http.HttpRequestMessageExtensions+<CreateNewAsync>d__0" fullword ascii 
   condition:
      uint16(0) == 0x5a4d and filesize < 200KB and
      1 of ($x*) and 4 of them
}