# =============================================================================
#  Multi-stage Dockerfile
# =============================================================================
#  Usage:
#    docker build -t portfolio:local . && docker run --rm portfolio:local
# =============================================================================

# -----------------------------------------------------------------------------
#  Build Stage
# -----------------------------------------------------------------------------
FROM golang:1.24-alpine3.21 AS build

ENV CGO_ENABLED=1

RUN apk add --no-cache \
    gcc  \
    g++  \
    make \
    git  \
    musl-dev

WORKDIR /workspace

COPY *.go /workspace/
COPY ./static /workspace/static
COPY ./data/serve/robots.txt /workspace/data/serve/robots.txt

RUN \
    go mod init webserver && \
    go mod tidy
RUN \
    GOOS=linux go build -ldflags="-s -w" -o ./bin/web-app ./

# -----------------------------------------------------------------------------
#  Main Stage
# -----------------------------------------------------------------------------
FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    sqlite

WORKDIR /app

COPY --from=build /workspace/bin/web-app /usr/local/bin/web-app
COPY --from=build /workspace/static ./static/
COPY --from=build /workspace/data ./data/
COPY ./bin/main /usr/local/bin/make-thumbnails

EXPOSE 80
ENTRYPOINT ["/usr/local/bin/web-app", "--port 80"]
