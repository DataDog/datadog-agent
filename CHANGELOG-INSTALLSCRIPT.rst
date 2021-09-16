=============
Release Notes
=============

.. _Release Notes_installscript-1.6.0:

installscript-1.6.0
===================

.. _Release Notes_installscript-1.6.0_Enhancement Notes:

Enhancement Notes
-----------------

- Suggest installing the IoT Agent on armv7l.


.. _Release Notes_installscript-1.6.0_Bug Fixes:

Bug Fixes
---------

- Ensure that Debian/Ubuntu APT keyrings get created world-readable, so that
  the ``_apt`` user can read them.

- Improved detection of systemd as init system.


.. _Release Notes_installscript-1.5.0:

installscript-1.5.0
===================

.. _Release Notes_installscript-1.5.0_New Features:

New Features
------------

- Adds capability to specify a minor (and optional patch) version by setting
  the ``DD_AGENT_MINOR_VERSION`` variable.


.. _Release Notes_installscript-1.5.0_Enhancement Notes:

Enhancement Notes
-----------------

- Adds email validation before sending a report.

- Improvements for APT keys management

  - By default, get keys from keys.datadoghq.com, not Ubuntu keyserver
  - Always add the ``DATADOG_APT_KEY_CURRENT.public`` key (contains key used to sign current repodata)
  - Add ``signed-by`` option to all sources list lines
  - On Debian >= 9 and Ubuntu >= 16, only add keys to ``/usr/share/keyrings/datadog-archive-keyring.gpg``
  - On older systems, also add the same keyring to ``/etc/apt/trusted.gpg.d``


.. _Release Notes_installscript-1.5.0_Bug Fixes:

Bug Fixes
---------

- Fix SUSE version detection algorithm to work without deprecated ``/etc/SuSE-release`` file.


.. _Release Notes_installscript-1.4.0:

installscript-1.4.0
===================

.. _Release Notes_installscript-1.4.0_Enhancement Notes:

Enhancement Notes
-----------------

-  Add a ``gpgkey=`` entry ensuring that ``dnf``/``yum``/``zypper``
   always have access to the key used to sign current repodata.

-  Change RPM key location from yum.datadoghq.com to keys.datadoghq.com.

-  Activate ``repo_gpgcheck`` on RPM repositories by default.
   ``repo_gpgcheck`` is still set to ``0`` when using a custom
   ``REPO_URL`` or when running on RHEL/CentOS 8.1 because of a `bug in
   dnf`_. The default value can be overriden by specifying
   ``DD_RPM_REPO_GPGCHECK`` variable. The allowed values are ``0`` (to
   disable) and ``1`` (to enable).

.. _bug in dnf: https://bugzilla.redhat.com/show_bug.cgi?id=1792506

.. _Release Notes_installscript-1.3.1:

1.3.1
===================

.. _Release Notes_installscript-1.3.1_Prelude:

Prelude
-------

Released on: 2021-02-22

.. _Release Notes_installscript-1.3.1_New Features:

New Features
------------

- Print script version in the logs.


.. _Release Notes_installscript-1.3.1_Bug Fixes:

Bug Fixes
---------

- On error, the user prompt will now only run when a terminal is attached.
  It will have a default negative answer and it will time out after 60 seconds.


.. _Release Notes_installscript-1.3.0:

1.3.0
===================

Prelude
-------

Released on: 2021-02-15

Bug Fixes
---------

- Fix installation on SUSE < 15.


1.2.0
===================

Prelude
-------

Released on: 2021-02-12

New Features
------------

- Add release notes for installer changes.

- Prompt user to open support case when there is a failure during installation.
