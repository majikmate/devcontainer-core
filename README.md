# devcontainer-core

The shared base of all Dev Container images. It has three parts:

1. **The image** `ghcr.io/majikmate/devcontainer-core`: Debian 13 (trixie) with the
   development user `dev`, zsh, locales, git settings, the Pure prompt and an SSH server.
2. **The layer tool `devcon`**: a static Go program (built with `CGO_ENABLED=0`) in
   the image. Every feature of every image is one layer, installed with
   `RUN devcon install <layer>`. It also writes the VS Code settings of the
   layers into the image label, so the image Dockerfiles do not need to handle them.
3. **The release tooling**: the shared workflow
   [`.github/workflows/devcontainer-image.yml`](.github/workflows/devcontainer-image.yml)
   and the Go program `devcon-release`. They decide when an image needs a new
   version, then build, test and release it.

Only GitHub native tools, Docker native tools (`docker buildx`) and our own Go
programs are used. There are no Dev Container features, no Dev Container CLI,
and no Bash or Python scripts.

## Image chain

```
devcontainer-core                   (this repository)
├── devcontainer-base               + go, node, deno, prettier
│   ├── devcontainer-classroom-web
│   ├── devcontainer-web-advanced   + playwright-deps
│   └── devcontainer-dev            + github-cli
└── devcontainer-classroom-exam-ts  + deno
```

## Writing an image

An image repository has a `.devcontainer/Dockerfile` that starts from core (or
from an image that builds on core) and adds layers:

```dockerfile
FROM ghcr.io/majikmate/devcontainer-core:1

# deno: the release workflow passes the newest version; without it the newest is installed
ARG DENO_VERSION
RUN devcon install deno
```

Rules:

- One `RUN devcon install <layer>` per layer, so every feature is one Docker layer.
- Declare the tool versions of a layer as `ARG` right before its `RUN` line
  (see [docs/layers.md](docs/layers.md) for the argument names).
- A layer checks that the layers it needs are installed and stops the build otherwise.

The `.devcontainer/devcontainer.json` of the image repository keeps the VS Code
settings of that image (extensions, settings). The release workflow writes them
into the image label `devcontainer.metadata`, together with the settings of all layers.

## Layers

All layers, their build arguments, tool versions and label entries are listed in
[docs/layers.md](docs/layers.md). The file is generated from the code:

```sh
go run ./cmd/devcon layers --markdown > docs/layers.md
```

The code of a layer is one file in [`internal/layers`](internal/layers). A layer declares:

| Field      | Meaning                                                                   |
| ---------- | ------------------------------------------------------------------------- |
| `Name`     | name used in `devcon install <name>`                                      |
| `Needs`    | layers that must be installed before                                      |
| `Args`     | build arguments with default values                                       |
| `Tools`    | tools with a version: build argument and a function for the newest version |
| `Metadata` | VS Code settings and container options for the label `devcontainer.metadata` |
| `Install`  | installation (runs as root during the build)                              |
| `Test`     | test in the built image (runs as the development user)                    |
| `Start`    | optional start step, run by `devcon start` when the container starts      |
| `Commands` | optional `devcon` commands of the layer                                   |

To add a layer: create a file in `internal/layers`, register the layer in its
`init` function, regenerate `docs/layers.md`, and release core.

### Framework packages

The framework is public, so that layers in other repositories can use it
(module `github.com/majikmate/devcontainer-core`):

| Package | Content |
| ------- | ------- |
| [`pkg/layer`](pkg/layer) | layer definition, registry, test helpers |
| [`pkg/sys`](pkg/sys) | commands, downloads with checksum, archives, users, files, groups |
| [`pkg/debian`](pkg/debian) | Debian-specific: apt, pending package updates |
| [`pkg/shellrc`](pkg/shellrc) | settings for bash and zsh |
| [`pkg/state`](pkg/state) | installed layers and development user of an image |
| [`pkg/devcontainer`](pkg/devcontainer) | entries of the label `devcontainer.metadata` |
| [`pkg/versions`](pkg/versions) | newest versions from go.dev, Node.js, Deno, npm, GitHub releases |

Distribution-independent layers use only `pkg/sys` for system work; only
Debian-bound layers use `pkg/debian`.

### The label `devcontainer.metadata`

The label is a JSON list. VS Code, Codespaces and the Dev Container tools merge
its entries in order. The release workflow creates the list:

1. the entries of the base image (from its label),
2. one entry per new layer, with the id `devcon/<layer>` (from `devcon metadata`),
3. one entry for the image, with the id `devcon/image/<repository>` (from its `devcontainer.json`).

When an installed layer has a start step, `devcon metadata` also adds the
entry `devcon/start` with `"postStartCommand": "devcon start"` (once for all
layers). Entries with the same id are not added twice.

## devcon commands

| Command                    | Where                    | What it does                                             |
| -------------------------- | ------------------------ | -------------------------------------------------------- |
| `devcon install <layer>…`  | Dockerfile (root)        | installs layers                                          |
| `devcon layers [--markdown]` | anywhere               | lists the layers (`*` = installed in this image)         |
| `devcon test [<layer>…]`   | built image              | tests the installed layers                               |
| `devcon metadata`          | built image              | prints the label entries of the installed layers         |
| `devcon start [<cmd>…]`    | container start          | runs the start steps of the installed layers, then `<cmd>` |
| `devcon ssh-keys`          | container (layer sshd)   | loads the SSH keys of the owner's GitHub account         |
| `devcon sshd-start`        | container, root (layer sshd) | starts the SSH server                                |
| `devcon os-updates [--security]` | root (layer os)    | lists pending Debian updates without installing them (used by the security check) |

The images do not update Debian packages when a container is created. The
release workflow keeps the images current instead (see below).

## SSH access

The SSH server listens on port **2222**. It accepts only public keys: no password
login and no root login. The keys are the public keys of the GitHub account of the
container owner (`https://github.com/<user>.keys`). They replace
`~/.ssh/authorized_keys` of the user `dev`. When GitHub cannot be reached, the
existing keys stay.

`devcon` finds the GitHub user name in this order:

1. the environment variable `GITHUB_USER`,
2. the file `/workspaces/.codespaces/shared/.env` (Codespaces),
3. the git setting `github.user` in `~/.gitconfig` (the Dev Containers extension
   copies the `.gitconfig` of your computer into the container).

| Environment                 | Start of SSH server and key loading                          | Where the user name comes from |
| --------------------------- | ------------------------------------------------------------ | ------------------------------ |
| Codespaces                  | `postStartCommand` and `postAttachCommand` of the label      | `GITHUB_USER` (set by Codespaces) |
| Dev Containers extension    | `postStartCommand` and `postAttachCommand` of the label      | `github.user` in your `~/.gitconfig` |
| `docker run` (local or on a VM) | `ENTRYPOINT ["devcon", "start"]`                         | `-e GITHUB_USER=<user>`        |

Setup for the Dev Containers extension (once, on your computer):

```sh
git config --global github.user <your-github-user>
```

Example with Docker on your computer or on a VM:

```sh
docker run -d --name dev -p 2222:2222 -e GITHUB_USER=<your-github-user> \
  ghcr.io/majikmate/devcontainer-base:2
ssh -p 2222 dev@localhost
```

## Releases

Every image repository has a `.github/workflows/release.yml` that calls the shared
workflow of this repository:

```yaml
on:
  workflow_dispatch:
    inputs:
      # Every release.yml declares these three inputs (the chain build passes them)
      upstream:
        description: First update the upstream images (chain build)
        type: boolean
        default: true
      force:
        type: boolean
        default: false
      bump:
        type: choice
        options: [auto, patch, minor, major]
        default: auto

jobs:
  image:
    uses: majikmate/devcontainer-core/.github/workflows/devcontainer-image.yml@main
    with:
      image-title: …
      image-description: …
      major-version: 2
      # The image in the FROM line of .devcontainer/Dockerfile
      upstream-repositories: devcontainer-base
      update-upstream: ${{ github.event_name == 'workflow_dispatch' && inputs.upstream == true }}
      force: ${{ inputs.force || false }}
      bump: ${{ inputs.bump || 'auto' }}
    secrets:
      app-client-id: ${{ secrets.DEVCONTAINER_APP_CLIENT_ID }}
      app-private-key: ${{ secrets.DEVCONTAINER_APP_PRIVATE_KEY }}
```

The jobs run `devcon-release` with `go run`:

| Job        | Command                     | What it does                                                        |
| ---------- | --------------------------- | ------------------------------------------------------------------- |
| `upstream` | `devcon-release upstream`   | manual chain build: runs the Release workflows of the upstream images and waits |
| `prepare`  | `devcon-release plan`       | collects the inputs, decides whether to build and release, computes the version |
| `build`    | `devcon-release build`      | builds with `docker buildx`, sets the labels, tests, pushes (amd64 and arm64) |
| `publish`  | `devcon-release publish`    | creates the multi-architecture tags and the GitHub release          |

### When a new version is released

The plan compares the **inputs** of the newest release (label `devcon.inputs`)
with the current inputs:

- `config`: the git tree of the configuration paths (for core also the Go source),
- `image/<ref>`: the digest of every base image in the Dockerfile,
- `tool/<name>`: the newest version of every tool of the layers in the Dockerfile.

A new version is released when:

- an input changed;
- **Debian security updates** are pending for the image (see below);
- the newest image is older than `max-age-days` (default 7 days, for the other
  Debian updates);
- or with `force`.

**Security check.** Every nightly and manual run starts the newest published
image as root and runs `devcon os-updates --security`, which simulates an
upgrade (`apt-get -s dist-upgrade`) and lists the packages with a newer
version from the Debian security archive. Nothing is installed. The release
reason and the release notes name the packages. An image that installs the
layer `os` (core) counts all security updates, because its build upgrades all
packages. Any other image counts only the updates that its `FROM` image does
not have too: the other updates are the job of the base image, and the image
follows through the new base image digest.

Version step (`bump: auto`): minor when Go 1.x changes or the major version of
Node.js or Deno changes, otherwise patch. The major version is set in
`release.yml` (`major-version`).

Tags: `X.Y.Z`, `X.Y`, `X` and `latest`. Pull requests build and test the image
without a release.

### Schedule and chain build

The nightly checks run in chain order (UTC): core 23:17, base 01:17,
classroom-exam-ts 01:27, dev 03:37, classroom-web 03:47, web-advanced 03:57.

A manual run ("Run workflow" with the option `upstream`, on by default) builds
the whole chain below the image. The chain build is recursive: each image starts
the Release workflow of the image in its `FROM` line with `upstream=true` and
waits for it. Example for devcontainer-classroom-web:

```
classroom-web (manual start)
└─ starts base, waits
   └─ base starts core, waits
      └─ core: check, release if needed        (1st)
   └─ base: check, release if core changed      (2nd)
└─ classroom-web: check, release if base changed (3rd)
```

Every image in the chain runs the complete automatic check of
[When a new version is released](#when-a-new-version-is-released), the same as
in its nightly run:

- inputs: configuration, digest of the base image, newest tool versions,
- maximum age (`max-age-days`, Debian updates),
- version step (`bump: auto`: minor for a new Go 1.x or Node.js/Deno major, else patch).

So core releases when core needs it; base releases when core changed or base
needs it for its own reasons; the started image does the same. The options
`force` and `bump` of the manual start apply only to the started image; the
upstream images always run with `force=false` and `bump=auto`.
A failed run stops the chain. The nightly checks do not start the chain; they
run one after the other by their schedule.

The chain build needs the GitHub App `majikmate-devcontainer` (organization secrets
`DEVCONTAINER_APP_CLIENT_ID` and `DEVCONTAINER_APP_PRIVATE_KEY`, passed as
`app-client-id` and `app-private-key`) with the permission "Actions: write" on
the upstream repositories.

## Development

Build and test the Go programs (in any container with Go):

```sh
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
```

Build the image locally:

```sh
docker buildx build --load -t devcontainer-core:dev -f .devcontainer/Dockerfile .
docker run --rm --user dev --entrypoint devcon devcontainer-core:dev test
```

Show the digest and labels of a published image:

```sh
go run ./cmd/devcon-release inspect ghcr.io/majikmate/devcontainer-core:1
```

## License

MIT
