import "math"

rule JAR_Network_Indicators_Detection
{
  meta:
    description = "Detects JAR file info by hash"
    author = "TheKn0ck0ut"
    date = "2026-09-15"
	yarahub_uuid = "d107043d-e6de-48e4-96fe-353b39ef633d"
	yarahub_license = "CC0 1.0"
	yarahub_rule_matching_tlp = "TLP:WHITE"
	yarahub_rule_sharing_tlp = "TLP:WHITE"
	yarahub_reference_md5 = "e5ca35a3e8de02995c96a2e8972efb23"

  strings:
    // Matches IPv4 addresses
    $ip_indicator = /http:\/\/(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)/

    // Matches general external domains/URLs
    $url_regex = /(https?|ftp):\/\/([a-zA-Z0-9\-]+\.)+[a-zA-Z]{2,6}(\/[a-zA-Z0-9_\-\.~%]*)*/

    // Look for networking classes that indicate external traffic
    $java_net_1 = "java/net/URL" ascii wide
    $java_net_2 = "java/net/HttpURLConnection" ascii wide
    $java_net_3 = "java/net/Socket" ascii wide

  condition:
        // Has to match the strings and the
    uint16(0) == 0x4B50 and
    (any of ($java_net_1, $java_net_2, $java_net_3)) and
    ($ip_indicator or $url_regex)
}