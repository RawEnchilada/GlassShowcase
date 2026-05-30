FROM debian:bookworm-slim AS build

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY Makefile server.c tests.c ./
RUN make && make test

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        gosu \
        libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --create-home --home-dir /app glass

WORKDIR /app
COPY --from=build /app/glass-tower /app/glass-tower
COPY public /app/public
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN mkdir -p /app/data /data \
    && chown -R glass:glass /app /data \
    && chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 4300
ENV DATABASE_PATH=/data/ratings.sqlite3

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["/app/glass-tower"]
