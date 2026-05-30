FROM golang:1.24-bookworm AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o glass-tower .

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        gosu \
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