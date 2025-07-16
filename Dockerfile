FROM mirror.gcr.io/library/alpine:latest AS git-builder

SHELL ["sh", "-euxo", "pipefail", "-c"]
RUN --mount=type=cache,target=/var/cache/apk,sharing=locked \
    apk add gcc g++ libc-dev patch make autoconf automake libtool \
    perl curl curl-dev curl-static expat-dev expat-static openssl-dev \
    openssl-libs-static pcre2-dev perl-dev zlib-dev zlib-static

WORKDIR /usr/local/src/git

ARG GIT_VERSION=2.50.1

RUN curl -f#L "https://www.kernel.org/pub/software/scm/git/git-$GIT_VERSION.tar.xz" | \
      tar -xJ --strip-components=1; \
    ./configure CFLAGS="-Os -pipe -flto" LDFLAGS="-static-pie -flto -flto-partition=one" --with-libpcre --without-tcltk; \
    make -j$(nproc) git; \
    install -s -t / git

FROM mirror.gcr.io/library/golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go .
COPY pkg ./pkg/
COPY internal ./internal/

# Statically comple with CGO enabled will be needed if we integrate go-tree-sitter
# RUN GOOS=linux go build  --ldflags '-extldflags "-static"' -v -o codeowners main.go

# Statically compile with CGO disabled
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -v -o codeowners main.go

FROM mirror.gcr.io/library/busybox:1-musl

COPY --from=git-builder /git /usr/local/bin/
COPY --from=builder /app/codeowners /usr/local/bin/
COPY entrypoint.sh /

ENTRYPOINT ["/entrypoint.sh"]
