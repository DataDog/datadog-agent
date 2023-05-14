FROM datadog/docker-library:python_2_7-alpine3_6

RUN pip install datadog

COPY sender.py /sender.py

CMD [ "python", "/sender.py" ]
