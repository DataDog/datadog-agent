FROM jasonrivers/nagios:latest
COPY --chown=nagios:nagios nagios.cfg /opt/nagios/etc/nagios.cfg
COPY --chown=nagios:nagios mysql.cfg /opt/nagios/etc/conf.d/mysql.cfg
RUN  chown -R nagios:nagios /opt/nagios/var/
