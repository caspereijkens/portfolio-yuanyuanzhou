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
COPY ./ops /workspace/ops
COPY ./static /workspace/static
COPY ./data/serve/robots.txt /workspace/data/serve/robots.txt

RUN \
    go mod init webserver && \
    go mod tidy
RUN \
    GOOS=linux go build -ldflags="-s -w" -o ./bin/web-app ./
RUN \
    GOOS=linux go build -ldflags="-s -w" -o ./bin/make-thumbnails ./ops/make-thumbnails/main.go  
RUN \
    GOOS=linux go build -ldflags="-s -w" -o ./bin/cleanup-filepaths ./ops/cleanup-filepaths/main.go

# -----------------------------------------------------------------------------
#  Main Stage
# -----------------------------------------------------------------------------
FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    sqlite

WORKDIR /app

COPY --from=build /workspace/bin/web-app /usr/local/bin/web-app
COPY --from=build /workspace/bin/make-thumbnails /usr/local/bin/make-thumbnails
COPY --from=build /workspace/bin/cleanup-filepaths /usr/local/bin/cleanup-filepaths
COPY --from=build /workspace/static ./static/
COPY --from=build /workspace/data ./data/

EXPOSE 80
ENTRYPOINT ["/usr/local/bin/web-app", "--port 80"]
