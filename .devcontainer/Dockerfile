# devcontainer-core: the base of all Dev Container images.
#
# Debian 13 with the development user "dev", zsh, locales, git settings, the
# Pure prompt and the SSH server. It contains the program devcon, which installs
# the layers; every image that builds on this image uses it:
#
#   FROM ghcr.io/majikmate/devcontainer-core:1
#   ARG DENO_VERSION
#   RUN devcon install deno
#
# The release workflow passes the tool versions as build arguments and sets the
# labels (OCI and devcontainer.metadata).

# ── devcon: built from source, static (no C libraries) ───────────────────────
# The framework and the Debian-bound layers come from this repository, the
# distribution-independent layers from devcontainer-features.
FROM golang:1.27-trixie AS devcon
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
# features: the distribution-independent layers of devcontainer-features, in
# the version that the release workflow found (the newest without it)
ARG FEATURES_VERSION=latest
RUN go get github.com/majikmate/devcontainer-features@${FEATURES_VERSION}
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/devcon ./cmd/devcon

# ── image ────────────────────────────────────────────────────────────────────
FROM buildpack-deps:trixie-curl
COPY --from=devcon /out/devcon /usr/local/bin/devcon

# os: Debian upgrade and basic tools
RUN devcon install os

# user: "dev" (1000:1000) with zsh and sudo
ARG USERNAME=dev
ARG USER_UID=1000
ARG USER_GID=1000
RUN devcon install user

# locales: language en_US.UTF-8, regional formats de_AT.UTF-8, time zone Europe/Vienna
ENV LANG=en_US.UTF-8 \
    LANGUAGE=en_US:en \
    LC_TIME=de_AT.UTF-8 \
    LC_NUMERIC=de_AT.UTF-8 \
    LC_MONETARY=de_AT.UTF-8 \
    LC_MEASUREMENT=de_AT.UTF-8 \
    TZ=Europe/Vienna
RUN devcon install locales

# git: rebase on pull, auto stash
RUN devcon install git

# aliases: ls, ll, grep, vs
RUN devcon install aliases

# pure-prompt: Pure prompt for zsh
ARG PURE_VERSION
RUN devcon install pure-prompt

# sshd: SSH server on port 2222, keys of the owner's GitHub account
RUN devcon install sshd
EXPOSE 2222

# docker run: start the SSH server and load the keys, then keep running.
# (The Dev Containers extension and Codespaces use postStartCommand instead.)
ENTRYPOINT ["devcon", "start"]
CMD ["sleep", "infinity"]
