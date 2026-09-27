# devcontainer-core

The shared base of all Dev Container images. It has four parts:

1. **The image** `ghcr.io/majikmate/devcontainer-core`: Debian 13 "trixie"
   with the user `dev`, zsh, locales, git settings, the Pure prompt and an SSH
   server.
2. **The layer tool `devcon`**: a static Go program (`CGO_ENABLED=0`) in the
   image. Every part of every image is one layer, installed with
   `RUN devcon install <layer>`. `devcon` also provides the VS Code settings of
   the layers.
3. **The Debian-bound layers** ([`pkg/layers`](pkg/layers)): os, user,
   locales, sshd, build-tools, playwright-deps. The distribution-independent
   layers are in
   [devcontainer-features](https://github.com/majikmate/devcontainer-features).
4. **The release tooling**: the shared workflows in
   [`.github/workflows`](.github/workflows) and the Go program
   `devcon-release`. They decide when an image needs a new version, then
   build, test, release and prune it.

Only GitHub native tools, Docker native tools (`docker buildx`) and our own Go
programs are used: no Dev Container features, no Dev Container CLI, no Bash or
Python scripts.

**Image:** `ghcr.io/majikmate/devcontainer-core:1` · linux/amd64, linux/arm64 ·
[release notes](https://github.com/majikmate/devcontainer-core/releases)

## Dependencies

```text
                                               Nightly Content
devcontainer-features                                  Go library of layers, compiled into devcon
  ▼
devcontainer-core:1                            23:17   Debian 13, devcon, user dev, zsh, SSH server
├── devcontainer-base:2                        01:17   + go, build-tools, node, deno, prettier
│   ├── devcontainer-dev:2                     03:37   + github-cli
│   ├── devcontainer-classroom-web:2           03:47   classroom settings, AI off
│   └── devcontainer-classroom-web-advanced:2  03:57   + playwright-deps, AI on
└── devcontainer-classroom-exam-ts:2           01:27   + deno, AI and coding assistance off
```

This repository: **devcontainer-core**. Nightly checks in UTC. Repositories:
[core](https://github.com/majikmate/devcontainer-core) ·
[features](https://github.com/majikmate/devcontainer-features) ·
[base](https://github.com/majikmate/devcontainer-base) ·
[dev](https://github.com/majikmate/devcontainer-dev) ·
[classroom-web](https://github.com/majikmate/devcontainer-classroom-web) ·
[classroom-web-advanced](https://github.com/majikmate/devcontainer-classroom-web-advanced) ·
[classroom-exam-ts](https://github.com/majikmate/devcontainer-classroom-exam-ts)

## Content

Each layer is one line in [`.devcontainer/Dockerfile`](.devcontainer/Dockerfile):

| Layer | Content | Version |
| ----- | ------- | ------- |
| `os` | all Debian updates and the basic tools | Debian 13 "trixie" |
| `user` | user `dev` (UID 1000) with zsh and sudo | — |
| `locales` | locales of `LANG` and `LC_*`, time zone `TZ` | Debian packages |
| `git` | system-wide git settings (rebase on pull, auto stash) | — |
| `aliases` | shell aliases: ls, ll, grep, vs | — |
| `pure-prompt` | Pure prompt for zsh | newest release |
| `sshd` | SSH server on port 2222 (see [SSH access](#ssh-access)) | Debian packages |

`devcon` is built with the newest version of devcontainer-features (build
argument `FEATURES_VERSION`). A new features version leads to a new core image;
the other images follow through the new core image digest.

## Writing an image

The `.devcontainer/Dockerfile` of an image starts from core (or from an image
that builds on core) and adds layers:

```dockerfile
FROM ghcr.io/majikmate/devcontainer-core:1

ARG DENO_PIN=2
ARG DENO_VERSION
RUN devcon install deno
```

- One `RUN devcon install <layer>` per layer, so every layer is one Docker
  layer.
- Declare the build arguments of a layer as `ARG` right before its `RUN` line
  ([docs/layers.md](docs/layers.md)). The release workflow passes the newest
  tool versions; without them, the layer installs the newest version.
- A layer checks that the layers it needs are installed.
- The `.devcontainer/devcontainer.json` keeps the VS Code settings of the
  image. The release workflow writes them, together with the settings of all
  layers, into the image label `devcontainer.metadata`.

**Pinned release lines.** `ARG <TOOL>_PIN=<line>` in the Dockerfile that
installs the layer pins a release line (Go, Node.js, Deno; rules in
[devcontainer-features](https://github.com/majikmate/devcontainer-features#pinned-release-lines)).
The layer installs the newest release inside the line. At the end of life of
the line, the release check and the build fail; there is no warning before.
The message names the line, the reason, the source and the supported lines:

```text
go 1.27 (GO_PIN=1.27) has reached its end of life (Go 1.29.0 was released; Go supports the two newest major releases).
Source: https://go.dev/doc/devel/release#policy. Change ARG GO_PIN in the Dockerfile to a supported version (supported: 1.28, 1.29).
```

The Debian release has the same check (layer `os`): when its regular security
support ends (column `eol` of `distro-info-data`), the build and the nightly
check of core fail. The fix is a newer Debian release in the `FROM` lines of
the core Dockerfile.

## Layers

All layers with their build arguments, tool versions and label entries:
[docs/layers.md](docs/layers.md), generated with
`go run ./cmd/devcon layers --markdown > docs/layers.md`.

A layer is one Go file that registers itself in its `init` function: in
[`pkg/layers`](pkg/layers) when it needs the package manager or other parts of
the distribution, otherwise in devcontainer-features. It declares:

| Field | Meaning |
| ----- | ------- |
| `Name` | name used in `devcon install <name>` |
| `Needs` | layers that must be installed before |
| `Args` | build arguments with default values |
| `Tools` | tools with a build argument, a function for the newest version and an optional pin |
| `Metadata` | VS Code settings and container options for the label `devcontainer.metadata` |
| `Install` | installation (root, during the build) |
| `Test` | test in the built image (user `dev`) |
| `Check` | optional support check (end of life), run by `devcon check` |
| `Start` | optional start step, run by `devcon start` |
| `Commands` | optional `devcon` commands of the layer |

**Framework packages** (public, module `github.com/majikmate/devcontainer-core`):

| Package | Content |
| ------- | ------- |
| [`pkg/layer`](pkg/layer) | layer definition, registry, pinned release lines, test helpers |
| [`pkg/sys`](pkg/sys) | commands, downloads with checksum, archives, users, files |
| [`pkg/debian`](pkg/debian) | apt, pending package updates, Debian releases |
| [`pkg/shellrc`](pkg/shellrc) | settings for bash and zsh |
| [`pkg/state`](pkg/state) | installed layers and development user of an image |
| [`pkg/devcontainer`](pkg/devcontainer) | entries of the label `devcontainer.metadata` |
| [`pkg/versions`](pkg/versions) | newest versions from go.dev, Node.js, Deno, npm, GitHub releases |
| [`pkg/layers`](pkg/layers) | the Debian-bound layers |

**The label `devcontainer.metadata`** is a JSON list that VS Code and
Codespaces merge in order: the entries of the base image, one entry per new
layer (id `devcon/<layer>`), one entry for the image (id
`devcon/image/<repository>`), and `devcon/start` with
`"postStartCommand": "devcon start"` when a layer has a start step.

## devcon commands

| Command | Where | What it does |
| ------- | ----- | ------------ |
| `devcon install <layer>…` | Dockerfile (root) | installs layers |
| `devcon layers [--markdown]` | anywhere | lists the layers (`*` = installed) |
| `devcon test [<layer>…]` | built image | tests the installed layers |
| `devcon metadata` | built image | prints the label entries of the installed layers |
| `devcon check [<layer>…]` | built image | checks the support of the installed layers (end of life) |
| `devcon start [<cmd>…]` | container start | runs the start steps, then `<cmd>` |
| `devcon ssh-keys` | container (layer sshd) | loads the SSH keys of the owner's GitHub account |
| `devcon sshd-start` | container, root (layer sshd) | starts the SSH server |
| `devcon os-updates [--security]` | root (layer os) | lists pending Debian updates without installing them |

## SSH access

The SSH server listens on port **2222** and accepts only the public keys of the
GitHub account of the container owner (`https://github.com/<user>.keys`): no
password, no root login. When GitHub cannot be reached, the existing keys stay.

| Environment | Where the GitHub user name comes from |
| ----------- | ------------------------------------- |
| Codespaces | `GITHUB_USER` (set by Codespaces) |
| Dev Containers extension | `git config --global github.user <your-github-user>` on your computer (run once) |
| `docker run` (local or on a VM) | `-e GITHUB_USER=<your-github-user>` |

```sh
docker run -d --name dev -p 2222:2222 -e GITHUB_USER=<your-github-user> \
  ghcr.io/majikmate/devcontainer-base:2
ssh -p 2222 dev@localhost
```

## Releases

Every image repository has a `.github/workflows/release.yml` that calls the
shared workflow of this repository:

```yaml
jobs:
  image:
    uses: majikmate/devcontainer-core/.github/workflows/devcontainer-image.yml@main
    with:
      image-title: …
      image-description: …
      major-version: 2
      upstream-repositories: devcontainer-base # the image in the FROM line
      update-upstream: ${{ github.event_name == 'workflow_dispatch' && inputs.upstream == true }}
      force: ${{ inputs.force || false }}
      bump: ${{ inputs.bump || 'auto' }}
    secrets:
      app-client-id: ${{ secrets.DEVCONTAINER_APP_CLIENT_ID }}
      app-private-key: ${{ secrets.DEVCONTAINER_APP_PRIVATE_KEY }}
```

| Job | Command | What it does |
| --- | ------- | ------------ |
| `upstream` | `devcon-release upstream` | manual chain build: runs the Release workflows of the upstream images and waits |
| `prepare` | `devcon-release plan` | collects the inputs, decides whether to release, computes the version |
| `build` | `devcon-release build` | builds with `docker buildx`, sets the labels, tests, pushes (amd64 and arm64) |
| `publish` | `devcon-release publish` | creates the multi-architecture tags and the GitHub release |
| `prune` | `devcon-release prune` | deletes the outdated versions of the image package |

### When a new version is released

The plan compares the inputs of the newest release (label `devcon.inputs`) with
the current inputs:

- `config`: the git tree of the configuration paths (for core also the Go
  source),
- `image/<ref>`: the digest of every base image in the Dockerfile,
- `tool/<name>`: the newest version of every tool of the layers in the
  Dockerfile (inside the pinned line).

First, the plan checks the pinned lines and runs `devcon check` in the newest
image; an end of life stops the run. A new version is released when:

- an input changed;
- **Debian updates** are pending: the plan runs `devcon os-updates` in the
  newest image (a simulated upgrade; nothing is installed). Core counts all
  updates; any other image counts only the updates that its `FROM` image does
  not have too;
- the newest image is older than `max-age-days` (default 7 days);
- or with `force`.

**Version step** (`bump: auto`): minor when Go 1.x or the major version of
Node.js or Deno changes, otherwise patch; the major version is `major-version`.
**Tags:** `X.Y.Z`, `X.Y`, `X` and `latest`. Pull requests build and test
without a release.

### Kept package versions

After every run (not for pull requests), the job `prune` deletes the outdated
versions of the image package on ghcr.io:

- releases of the current major line older than `prune-max-age-days` (default
  90 days), with their `-amd64` and `-arm64` versions,
- versions of major lines below `major-version`,
- untagged versions that no kept image uses, and the old `buildcache-*` tags.

Always kept: the newest release, every version with a moving tag (`2`, `2.0`,
`latest`) and the parts of kept images. When the references of a kept image
cannot be read, no untagged version is deleted. The input `prune` selects
`apply` (default), `report` (list only) or `off`. Deleting cannot be undone.

Every repository also has the manual workflow **Actions → Prune** (modes
`report` and `apply`; scopes `outdated` and `all-but-newest`, which deletes
every release except the newest). It calls the shared workflow
[`devcontainer-prune.yml`](.github/workflows/devcontainer-prune.yml).

### Schedule and chain build

The nightly checks run in chain order, two hours after the image they build on
(times in [Dependencies](#dependencies)). They do not start each other.

A manual run (**Actions → Release → Run workflow**, option `upstream` on by
default) first starts the Release workflow of the image in its `FROM` line and
waits; that image does the same. Example for devcontainer-classroom-web:

```text
classroom-web (manual start)
└─ starts base, waits
   └─ base starts core, waits
      └─ core: check, release if needed        (1st)
   └─ base: check, release if core changed      (2nd)
└─ classroom-web: check, release if base changed (3rd)
```

Every image runs its complete automatic check. `force` and `bump` apply only
to the started image. A failed run stops the chain. The chain build needs the
GitHub App `majikmate-devcontainer` (organization secrets
`DEVCONTAINER_APP_CLIENT_ID` and `DEVCONTAINER_APP_PRIVATE_KEY`) with
"Actions: write" on the upstream repositories.

## Development

```sh
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
docker buildx build --load -t devcontainer-core:dev -f .devcontainer/Dockerfile .
docker run --rm --user dev --entrypoint devcon devcontainer-core:dev test
go run ./cmd/devcon-release inspect ghcr.io/majikmate/devcontainer-core:1
```

## License

MIT
