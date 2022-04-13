FROM public.ecr.aws/lambda/nodejs:14

ENV AWS_LAMBDA_FUNCTION_TIMEOUT=3
ENV DD_API_KEY=xxxx
# DEBUG log level is required to make sure the 'agent start' timestamp is output
ENV DD_LOG_LEVEL=DEBUG
RUN mkdir -p /opt/extensions
COPY datadog-agent /opt/extensions/datadog-agent
COPY app.js /var/task/
COPY run.sh /var/task/

ENTRYPOINT [ "/var/task/run.sh" ]