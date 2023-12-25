FROM ubuntu:22.04

COPY init.sh /init.sh

# REPLACE:
# CMD /init.sh
# BY:

COPY cws-instrumentation /cws-instrumentation
CMD /cws-instrumentation trace -- /init.sh
