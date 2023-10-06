FROM datadog/docker-library:python_2_7-alpine3_6

RUN pip install --no-cache-dir datadogpy
COPY sender.py /sender.py

CMD [ "python", "/sender.py" ]
