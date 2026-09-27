# check=skip=InvalidDefaultArgInFrom
# (DEBIAN_SERIES has no default on purpose: the release workflow passes it.)
#
# devcontainer-core: the base of all Dev Container images.
#
# Debian (trixie, layer os) with the development user "dev", zsh, locales, git settings, the
# Pure prompt and the SSH server. It contains the program devenv, which installs
# the layers; every image that builds on this image uses it:
#
#   FROM ghcr.io/majikmate/devcontainer-core:1
#   ARG DENO_VERSION
#   RUN devenv install deno
#
# The release workflow passes the tool versions as build arguments and sets the
# labels (OCI and devcontainer.metadata). This file never decides a version:
# the layers do (and the devcontainer.json of an image can override them).

# The Debian release (series, for example trixie): the tool debian of the
# layer os decides it; the release workflow passes it.
ARG DEBIAN_SERIES

# ── devenv: built from source, static (no C libraries) ───────────────────────
# The framework and the Debian-bound layers come from this repository, the
# distribution-independent layers from devcontainer-features.
# The Go toolchain is the newest release of the Go line in go.mod (for
# example 1.27 → 1.27.1; the release workflow resolves it and stops at the end
# of life of the line). It is not the Go of the feature go. The Go package of
# Debian only starts the build: GOTOOLCHAIN makes it download that toolchain
# from the Go module proxy and check it against the Go checksum database.
FROM debian:${DEBIAN_SERIES} AS devenv
RUN apt-get update \
 && apt-get install -y --no-install-recommends golang-go ca-certificates \
 && rm -rf /var/lib/apt/lists/*
ARG DEVENV_GO_VERSION
ENV GOTOOLCHAIN=go${DEVENV_GO_VERSION}
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
# features: the distribution-independent layers of devcontainer-features, in
# the version that the release workflow found (the newest without it)
ARG FEATURES_VERSION=latest
RUN go get github.com/majikmate/devcontainer-features@${FEATURES_VERSION}
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/devenv ./cmd/devenv

# ── image ────────────────────────────────────────────────────────────────────
# The Debian image; the layer os installs the basic tools.
FROM debian:${DEBIAN_SERIES}
COPY --from=devenv /out/devenv /usr/local/bin/devenv

# os: Debian upgrade and basic tools
RUN devenv install os

# user: "dev" (1000:1000) with zsh and sudo
ARG USERNAME=dev
ARG USER_UID=1000
ARG USER_GID=1000
RUN devenv install user

# locales: language en_US.UTF-8, regional formats de_AT.UTF-8, time zone Europe/Vienna
ENV LANG=en_US.UTF-8 \
    LANGUAGE=en_US:en \
    LC_TIME=de_AT.UTF-8 \
    LC_NUMERIC=de_AT.UTF-8 \
    LC_MONETARY=de_AT.UTF-8 \
    LC_MEASUREMENT=de_AT.UTF-8 \
    TZ=Europe/Vienna
RUN devenv install locales

# git: rebase on pull, auto stash
RUN devenv install git

# aliases: ls, ll, grep, vs
RUN devenv install aliases

# pure-prompt: Pure prompt for zsh
ARG PURE_VERSION
RUN devenv install pure-prompt

# sshd: SSH server on port 2222, keys of the owner's GitHub account
RUN devenv install sshd
EXPOSE 2222

# docker run: start the SSH server and load the keys, then keep running.
# (The Dev Containers extension and Codespaces use postStartCommand instead.)
ENTRYPOINT ["devenv", "start"]
CMD ["sleep", "infinity"]
