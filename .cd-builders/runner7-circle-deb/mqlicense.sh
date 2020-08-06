#!/bin/bash
LAP_JAR=LAPApp.jar
MQVRMF=8.0.0.4
BUILD_PLATFORM=Linux_x86_64
UNAME_FLAG=-i
#############################################################################
#
#   <copyright
#   notice="lm-source-program"
#   pids="5724-H72"
#   years="2005,2015"
#   crc="1956258177" >
#   Licensed Materials - Property of IBM
#
#   5724-H72
#
#   (C) Copyright IBM Corp. 2005, 2015 All Rights Reserved.
#
#   US Government Users Restricted Rights - Use, duplication or
#   disclosure restricted by GSA ADP Schedule Contract with
#   IBM Corp.
#   </copyright>
#
#
# NAME: mqlicense
#
# PURPOSE: Launch Java License Agreement Process tool
#
#############################################################################


PROGNAME=`basename $0`         # Program name
PROGPATH=`dirname $0`          # Working directory

#-----------------------------------------------------------------------#


# Display command syntax
usage ()
{
    echo "Usage: ${PROGNAME?} [-accept] [-text_only] [ -jre ( path_to_java | \"path_to_java java_options\" ) ] ][-h|-?]"
}

declinemsg()
{
cat << +++EOM+++

Agreement declined:  Installation will not succeed unless
the license agreement is accepted.

+++EOM+++
}

copyright()
{
if [ -f $PROGPATH/copyright ] ; then
     cat $PROGPATH/copyright
fi
}

errormsg()
{
cat << +++EOM+++

ERROR:  Installation will not succeed unless the license
        agreement can be accepted.

        If the error was caused by a display problem,
        read the license agreement file  (Lic_xx.txt, where
        xx represents your language ) in the 'licenses'
        directory of the installation media, and then
	run the following command:

            ${PROGNAME?} -accept

        Only use this command if you accept the license
        agreement.

        For other errors, contact your IBM support centre.

+++EOM+++
}


#-----------------------------------------------------------------------#
#                             Main program
#-----------------------------------------------------------------------#
typeset -i RH_Version
typeset -i SUSE_Version
typeset -i Ubuntu_Version
#-----------------------------------------------------------------------#
# Set umask so that chckinstall script can read files in tmp            #
# needed on Solaris where checkinstall/request scripts run as nobody    #
#-----------------------------------------------------------------------#
umask 022

# Script must be run as root
id | grep "uid=0" > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "ERROR:  You must be 'root' to run this script."
    exit 1
fi


# Process command-line
#The following condition works correctly in bash, dash, and ksh
#Alternatively, could use ## while [ "$(echo $1 | cut -c1)" = "-" ] ##
while [ "${1%%[!-]*}" = "-" ]
do
    case $1 in
        "-accept")
            STATUSARG="-t 5"         ;;
        "-text_only")
            DISPLAYARG="-text_only"  ;;
        "-jre")
            LAPJRE=$2
            USER_DEFINED_JRE="true"
            shift                    ;;
        "-h" | "-?")
            usage; exit 0            ;;
        *)
            usage; exit 1            ;;
    esac
    shift
done

copyright

# Work out package release - required for /tmp license location
  MQVRM=`echo ${MQVRMF} | awk -F. '{print $1"."$2"."$3}'`

# Check whether the license has already been accepted
if [ -r /tmp/mq_license_${MQVRM}/license/status.dat ]; then
    echo "License has already been accepted:  Proceed with install."
    exit 0
fi


# Set JRE location
LAPJRE="${LAPJRE:-$(find $PROGPATH/lap -type d -name bin)/java}"
if [ ! -x "${LAPJRE}" ]; then

  if [ "${USER_DEFINED_JRE}" = "true" ]; then
    # The user specified a JRE which cannot be found/executed
    echo "ERROR: No executable Java program found at the specified -jre location: \"${LAPJRE}\""
    echo ""
    errormsg
    exit 1
  fi

  # If the installation image has been copied from one location to another, then
  # the file permissions may have been altered such that 'java' is no longer
  # executable.  Output an informative message if this is the case.
  if [ ! -f "${LAPJRE}" ]; then
    # There is no 'java' binary,
    echo "ERROR: Unable to locate the Java binary required by this license acceptance"
    echo "       script.  Check that the installation media has been correctly extracted."
    echo ""
    errormsg
    exit 1
  else
    # There is a 'java' file, but it cannot be executed.
    echo "ERROR: Unable to execute the Java binary located on the filesystem at:"
    echo "       \"${LAPJRE}\""
    echo "       Check that the installation media is suitable for the system"
    echo "       architecture, and check that the permissions of the installation files"
    echo "       match that contained within the original installation media."
    echo ""
    errormsg
    exit 1
  fi
fi


# Set classpath
LAPCLASSPATH=${PROGPATH?}/lap/${LAP_JAR}:${PROGPATH?}/lap/jre/lib/rt.jar:${PROGPATH?}/lap/jre/lib/i18n.jar

# Record the hardware architecture type
HARDWARE_ARCH=$(uname ${UNAME_FLAG})

# Check for graphics (if required)
if [ \( -z "${STATUSARG}" \) -a \( -z "${DISPLAYARG}" \) ]; then

    # When "xset -q" is run on a ppc Linux box exporting the display to a x86
    # box, the command hangs.  Therefore use xdpyinfo on ppc
    if [ "$uname" = "Linux" ] ; then
      CHECK_X_CMD="xdpyinfo"
    else
      CHECK_X_CMD="xset -q"
    fi
    ${CHECK_X_CMD} > /dev/null 2>&1

    # Default to text mode if there were any errors
    if [ $? -ne 0 ]; then
        DISPLAYARG="-text_only"
    elif [ ! -z "${DISPLAY}" ]; then
        echo "Displaying license agreement on ${DISPLAY}"
    fi

fi

# RedHat AS 3 does not install a c++ compatible library by default, which is
# needed by the JRE on Linux zSeries (compat-libstdc++-7.2-2.95.3.80.s390.rpm
# in RHEL AS 3 on zSeries).  This is overcome by turning off the jitc compiler
# using an environment variable.
output=$(echo $HARDWARE_ARCH | grep s390)
if [ $? -eq 0 ] ; then
  export JAVA_COMPILER=NONE
fi

# Launch LAP tool
${LAPJRE?} -cp ${LAPCLASSPATH?} com.ibm.lex.lapapp.LAP -l ${PROGPATH?}/lap/licenses -s /tmp/mq_license_${MQVRM} ${STATUSARG} ${DISPLAYARG}
RC=$?


# Display appropriate completion message depending on LAP return code
case ${RC?} in
    "3")
        declinemsg                                                     ;;
    "9")
	echo ""
        echo "Agreement accepted:  Proceed with install."
	echo ""                                                        ;;
    *)
        errormsg; exit ${RC}                                           ;;
esac
