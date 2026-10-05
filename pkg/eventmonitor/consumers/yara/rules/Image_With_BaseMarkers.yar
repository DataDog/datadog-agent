rule Image_With_BaseMarkers
{
  meta:
    author      = "ShadowOpCode"
    description = "Immages containing the markers 'BaseStart' and '-BaseEnd' Crypter-And-Tools"
    date = "2025-08-19"
    yarahub_uuid = "078b405d-1ac1-4deb-abf9-1c920bd4d018"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "00000000000000000000000000000000"


  strings:
    // Marker (ASCII and UTF-16LE)
    $marker_start    = "BaseStart" ascii nocase
    $marker_end      = "-BaseEnd" ascii nocase

    // Magic numbers images (offset 0)
    $jpg     = { FF D8 FF }
    $png     = { 89 50 4E 47 0D 0A 1A 0A }
    $gif87   = "GIF87a"
    $gif89   = "GIF89a"
    $bmp     = "BM"
    $tiff_le = { 49 49 2A 00 }
    $tiff_be = { 4D 4D 00 2A }
    $ico     = { 00 00 01 00 }
    $jp2     = { 00 00 00 0C 6A 50 20 20 0D 0A 87 0A } // JPEG 2000
    $riff    = "RIFF"  // WEBP: RIFF....WEBP
    $webp    = "WEBP"

  condition:
    (
      $jpg at 0 or
      $png at 0 or
      $gif87 at 0 or $gif89 at 0 or
      $bmp at 0 or
      $tiff_le at 0 or $tiff_be at 0 or
      $ico at 0 or
      $jp2 at 0 or
      ( $riff at 0 and $webp at 8 )  // WEBP
    )
    and
      $marker_start
    and
      $marker_end
}