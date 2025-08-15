# syntax=docker/dockerfile:1

FROM scratch

ENV PORT=8080
ENV DB_PATH=/data/data.db

WORKDIR /

# Binary will be provided by GoReleaser's build context
COPY microapi /micro-api

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/micro-api"]
