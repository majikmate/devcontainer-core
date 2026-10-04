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
devcontainer-core:1                            22:17   Debian 13, devcon, user dev, zsh, SSH server
├── devcontainer-base:2                        23:17   + go, build-tools, node, deno, prettier
│   ├── devcontainer-dev:2                     23:57   + github-cli
│   ├── devcontainer-classroom-web:2           00:07   + html-validate, classroom settings, AI off
│   └── devcontainer-classroom-web-advanced:2  00:17   + playwright-deps, html-validate, AI on
└── devcontainer-classroom-exam-ts:2           23:47   + deno, AI and coding assistance off
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
| `os` | all Debian updates and the basic tools | Debian 13 "trixie" ([`debianPin`](pkg/layers/os.go#L31-L34)) |
| `user` | user `dev` (UID 1000) with zsh and sudo | — |
| `locales` | locales of `LANG` and `LC_*`, time zone `TZ` | Debian packages of the Debian release ([`debianPin`](pkg/layers/os.go#L31-L34)) |
| `git` | system-wide git settings (rebase on pull, auto stash) | — |
| `aliases` | shell aliases: ls, ll, grep, vs | — |
| `pure-prompt` | Pure prompt for zsh | newest release ([`purePin`](https://github.com/majikmate/devcontainer-features/blob/main/pureprompt/pureprompt.go#L28-L31)) |
| `sshd` | SSH server on port 2222 (see [SSH access](#ssh-access)) | Debian packages of the Debian release ([`debianPin`](pkg/layers/os.go#L31-L34)) |

`devcon` is built with the newest version of devcontainer-features (build
argument `FEATURES_VERSION`). A new features version leads to a new core image;
the other images follow through the new core image digest.

## Writing an image

The `.devcontainer/Dockerfile` of an image starts from core (or from an image
that builds on core) and adds layers:

```dockerfile
FROM ghcr.io/majikmate/devcontainer-core:1

ARG DENO_VERSION
RUN devcon install deno
```

- One `RUN devcon install <layer>` per layer, so every layer is one Docker
  layer.
- Declare the build arguments of a layer as `ARG` right before its `RUN` line
  ([docs/layers.md](docs/layers.md)). The release workflow passes the chosen
  tool versions (see [Versions](#versions)); without them, the layer chooses
  the version itself with the same rule.
- A layer checks that the layers it needs are installed.
- The `.devcontainer/devcontainer.json` keeps the VS Code settings of the
  image. The release workflow writes them, together with the settings of all
  layers, into the image label `devcontainer.metadata`.

## Versions

**A Dockerfile never decides a version.** One general rule
([`pkg/layer/version.go`](pkg/layer/version.go)) chooses the version of every
tool:

1. The **source** of the tool lists its releases, newest first (for example
   the npm registry, the GitHub releases, the Go module proxy; general sources
   in [`pkg/versions`](pkg/versions)).
2. The **feature decides** the release line (`pin`) and the release channel
   (`channel`) in `Tool.Version`. The values are constants `<tool>Pin` and
   `<tool>Channel` at the top of the layer file, directly after the imports
   (for example `debianPin` in [`pkg/layers/os.go`](pkg/layers/os.go#L31-L34),
   `denoPin` in `deno/deno.go` of devcontainer-features). Empty means the
   newest release of the default channel.
3. The `devcontainer.json` of the image that installs the layer **can
   override** it with the same keys (no image uses an override today):

   ```jsonc
   "customizations": {
     "devcon": {
       "deno": { "channel": "stable" },
       "prettier": { "pin": "3" }
     }
   }
   ```

   `"pin": ""` removes the pin of the feature. An override of a tool that the
   image does not install stops the release.
4. The version is the newest release in the channel and in the line. A tool
   that follows another tool (gopls follows go) gets the newest of these
   releases that works with the version of that tool.

The release notes show the result as inputs `tool/<name>`, `pin/<name>` and
`channel/<name>`.

| Tool | Layer | Constants in | Pin | Channel |
| ---- | ----- | ------------ | --- | ------- |
| `debian` | `os` (core) | [`pkg/layers/os.go`](pkg/layers/os.go#L31-L34) | `trixie` (Debian 13) | — |
| `go` | `go` (features) | [`golang/golang.go`](https://github.com/majikmate/devcontainer-features/blob/main/golang/golang.go#L49-L54) | `1.27` | — |
| `node` | `node` (features) | [`node/node.go`](https://github.com/majikmate/devcontainer-features/blob/main/node/node.go#L39-L44) | `24` | `lts` (or `current`) |
| `deno` | `deno` (features) | [`deno/deno.go`](https://github.com/majikmate/devcontainer-features/blob/main/deno/deno.go#L36-L39) | `2` | `lts` (or `stable`) |
| `golangci-lint` | `go` (features) | [`golang/golang.go`](https://github.com/majikmate/devcontainer-features/blob/main/golang/golang.go#L49-L54) | none: the newest release | — |
| `nvm` | `node` (features) | [`node/node.go`](https://github.com/majikmate/devcontainer-features/blob/main/node/node.go#L39-L44) | none: the newest release | — |
| `prettier`, `prettier-plugin-tailwindcss` | `prettier` (features) | [`prettier/prettier.go`](https://github.com/majikmate/devcontainer-features/blob/main/prettier/prettier.go#L32-L37) | none: the newest release | — |
| `gh` | `github-cli` (features) | [`githubcli/githubcli.go`](https://github.com/majikmate/devcontainer-features/blob/main/githubcli/githubcli.go#L30-L33) | none: the newest release | — |
| `pure` | `pure-prompt` (features) | [`pureprompt/pureprompt.go`](https://github.com/majikmate/devcontainer-features/blob/main/pureprompt/pureprompt.go#L28-L31) | none: the newest release | — |

gopls, dlv, staticcheck and govulncheck have no constants: they follow `go`
(the newest release that works with the installed Go, see
[`golang/versions.go`](https://github.com/majikmate/devcontainer-features/blob/main/golang/versions.go)).

**End of life.** A source can have a support rule (Go, Node.js, Deno,
Debian). At the end of life of a pinned line, the release check and the build
fail; there is no warning before. The message names the line, the reason,
the source and the supported lines:

```text
go 1.27 (pinned line) has reached its end of life (Go 1.29.0 was released; Go supports the two newest major releases).
Source: https://go.dev/doc/devel/release#policy. Change the pinned line of go (the pin constant at the top of the file
of its layer, or customizations.devcon.go.pin in the devcontainer.json of the image) to a supported version
(supported: 1.28, 1.29).
```

**Debian.** The tool `debian` of the layer `os` chooses the Debian release
(series). The release plan passes it as `DEBIAN_SERIES`, and the core
Dockerfile uses it in `FROM debian:${DEBIAN_SERIES}` (both stages; no other
prebuilt image).

**The Go toolchain that builds `devcon`** is not the Go of the feature `go`:
it is the newest release of the Go line in this repository's
[`go.mod`](go.mod) (`go 1.27` → the newest 1.27.x). The release plan resolves
it (input `tool/devcon-go`) and passes it as `DEVCON_GO_VERSION`; at the end
of life of the line (Go 1.29.0 released), the release stops. The build stage
installs the Go package of Debian only to start the build: `GOTOOLCHAIN`
makes it download that toolchain from the Go module proxy and check it
against the Go checksum database. The release tool itself runs with the same
line (`actions/setup-go` with `go-version-file: go.mod`). The layer
`os` also checks the installed release in the image (`devcon check os`).

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
| `Tools` | tools with a build argument, a version source and the release choice of the feature (pin, channel) |
| `Metadata` | VS Code settings and container options for the label `devcontainer.metadata` |
| `Install` | installation (root, during the build) |
| `Test` | test in the built image (user `dev`) |
| `Check` | optional support check (end of life), run by `devcon check` |
| `Start` | optional start step, run by `devcon start` |
| `Commands` | optional `devcon` commands of the layer |

**Framework packages** (public, module `github.com/majikmate/devcontainer-core`):

| Package | Content |
| ------- | ------- |
| [`pkg/layer`](pkg/layer) | layer definition, registry, version rule (sources, pins, channels), test helpers |
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
| `prune` | `devcon-release prune` | deletes the outdated versions of the image package and the old workflow runs |

### When a new version is released

The newest release is the image with the major tag of the current major line
(for example `devcontainer-base:2`), never `latest`. The plan compares its
inputs (label `nimblescape.devcon.inputs`) with the current inputs:

- `config`: the content of the configuration paths (input `config-paths`,
  default `.devcontainer` and `README.md`, because GitHub shows the README on
  the package page; for core also the Go source),
- `image/<ref>`: the digest of every base image in the Dockerfile (with the
  build arguments of the plan, for example `DEBIAN_SERIES`),
- `tool/<name>`: the version of every tool of the layers in the Dockerfile
  (see [Versions](#versions)),
- `pin/<name>` and `channel/<name>`: the pinned line and the channel of a
  tool, for example `pin/deno=2` and `channel/deno=lts`.

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
**Tags:** `X.Y.Z`, `X.Y`, `X` and `latest`. Images and `devcontainer.json`
files use the major tag (`FROM ghcr.io/majikmate/devcontainer-base:2`), not
`latest`. Pull requests build and test without a release.

### Kept package versions

After every run (not for pull requests), the job `prune` deletes the outdated
versions of the image package on ghcr.io:

- releases of the current major line older than `prune-max-age-days` (default
  90 days), with their `-amd64` and `-arm64` versions,
- versions of major lines below `major-version`,
- untagged versions that no kept image uses, and the old `buildcache-*` tags.

Always kept: the newest release, every version with a moving tag (`2`, `2.0`,
`latest`) and the parts of kept images. When the references of a kept image
cannot be read, no untagged version is deleted.

The same job deletes the finished **workflow runs** of the repository older
than `prune-max-age-days`, whatever their result (success, failure,
cancelled); only runs that are still running are kept (permission
`actions: write`). The input `prune`
selects `apply` (default), `report` (list only) or `off`. Deleting cannot be
undone.

Every repository also has the manual workflow **Actions → Prune** (modes
`report` and `apply`) with two scopes: `outdated` (the rules above) and
`all-but-newest`, which deletes every release except the newest and every
workflow run except the newest run of each workflow (a clean-up). It calls
the shared workflow
[`devcontainer-prune.yml`](.github/workflows/devcontainer-prune.yml) and
grants `packages: write` and `actions: write`. devcontainer-features has no
package: its Prune workflow runs every Sunday with `runs-only: true` and
deletes only its old workflow runs.

### Schedule and chain build

The nightly checks run in chain order (times in [Dependencies](#dependencies)):
core at 22:17 UTC (00:17 CEST), base one hour later, classroom-exam-ts 30
minutes after base, then dev, classroom-web and classroom-web-advanced every
10 minutes. They do not start each other. The cron times are in UTC, so in
winter (CET) every check runs one hour earlier in local time.

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

The license notices follow the licensing rules of the project: the root
[`LICENSE`](LICENSE), a three-line `SPDX-License-Identifier: MIT` header in
every authored file that can carry comments, and a license footer at the end
of every Markdown document. `devcon-release notices` checks them; the shared
workflow runs it in every image repository, and `go test` checks this
repository.

```sh
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
go run ./cmd/devcon-release notices --dir .
docker buildx build --load -t devcontainer-core:dev -f .devcontainer/Dockerfile .
docker run --rm --user dev --entrypoint devcon devcontainer-core:dev test
go run ./cmd/devcon-release inspect ghcr.io/majikmate/devcontainer-core:1
```

---

© 2026 Hannes Stauss (scalarion@nimblescape.com) · [MIT License](LICENSE).
