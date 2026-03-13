FROM ruby:3.0-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends build-essential sqlite3 pkg-config libyaml-dev git && \
    gem install rails -v 7.1.5.1 && \
    rails new /app/rails_hello --minimal && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app/rails_hello

ENV PORT=7777

CMD ["sh", "-c", "rails server -p $PORT -b 0.0.0.0"]
