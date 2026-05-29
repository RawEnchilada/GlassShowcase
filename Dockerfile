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
        libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --create-home --home-dir /app glass

WORKDIR /app
COPY --from=build /app/glass-tower /app/glass-tower
COPY public /app/public

RUN mkdir -p /app/data \
    && chown -R glass:glass /app

USER glass
EXPOSE 4300

CMD ["/app/glass-tower"]
