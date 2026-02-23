<?xml version="1.0"?>
<xsl:stylesheet version="1.0"
                xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:output omit-xml-declaration="yes" indent="yes"/>

   <xsl:template match="@*|node()">
    <xsl:copy>
      <xsl:apply-templates select="@*|node()"/>
    </xsl:copy>
  </xsl:template>

   <xsl:template match="/volume/name"><name>{imageName}</name></xsl:template>
   <xsl:template match="/volume/key"><key>{volumeKey}</key></xsl:template>
   <xsl:template match="/volume/capacity"><capacity unit='bytes'>10737418240</capacity></xsl:template>
   <xsl:template match="/volume/allocation"><allocation unit='bytes'>10000007168</allocation></xsl:template>
   <xsl:template match="/volume/physical"><physical unit='bytes'>10000000000</physical></xsl:template>
   <xsl:template match="/volume/target/path"><path>{volumePath}</path></xsl:template>
   <xsl:template match="/volume/target/permissions/owner"><owner>{user}</owner></xsl:template>
   <xsl:template match="/volume/target/permissions/group"><group>{libvirtGroup}</group></xsl:template>
</xsl:stylesheet>
