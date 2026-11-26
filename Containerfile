FROM docker.io/library/golang:1.27 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/alcazar-server ./server/cmd/example

FROM docker.io/library/alpine:3.22

RUN addgroup -S alcazar \
    && adduser -S -G alcazar alcazar \
    && mkdir -p /data \
    && chown alcazar:alcazar /data

WORKDIR /app
COPY --from=build --chown=alcazar:alcazar /out/alcazar-server /app/alcazar-server
COPY --from=build --chown=alcazar:alcazar /src/server/config /app/server/config

ENV APP_ENVIRONMENT=prd \
    SQLITE_DSN=/data/alcazar.db \
    BADGER_DIR=/data/alcazar

EXPOSE 8080
VOLUME ["/data"]
USER alcazar
ENTRYPOINT ["/app/alcazar-server"]
