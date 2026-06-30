# Harbour

[![CI](https://github.com/agent-harbour/harbour/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/agent-harbour/harbour/actions/workflows/ci.yml)

Run agents across your repositories in a controlled, shareable environment.

Like Docker Compose, but for agent harnesses.

- Run agents in a sandbox (Colima VM)
- Work across multiple repositories by default
- Define and share your harness (`AGENTS.md`, `skills/`)
- Keep your existing Docker workflow (via docker context)
- Supports Claude or Codex

## Install

```sh
brew tap agent-harbour/harbour
brew install agent-harbour/harbour/harbour
harbour help
```

Harbour provisions and runs an isolated Colima VM on the host.
Homebrew installs Colima automatically for the formula.

## Quick Start

1. Create your harness

   - `AGENTS.md` contains shared instructions
   - `skills/` contains optional custom skills that Harbour links into the selected agent's skills directory in the VM user's home
   - `apt-packages.txt` (optional) lists extra apt packages to install in the VM during `harbour provision`. One package per line; `#` starts a comment; blank lines are ignored.
   - `provision.sh` (optional) runs after apt packages are installed, as the VM user with `HOME` set. Use it for setup that apt cannot do, e.g. installing nvm. Runs on every `harbour provision`, so keep it idempotent.

   See https://github.com/agent-harbour/harbour-harness-template for an example.

2. Curate your workspace

Your "workspace" is the directory that Harbour mounts into the VM. It should contain the repos you want to work on, plus
your Harbour harness.

Example workspace:

   ```
   ~/git
   |-- harbour-harness
   |   |-- AGENTS.md
   |   `-- skills
   |-- my-org
   |   |-- front-end
   |   |-- backend
   |   `-- backend-hotfix-worktree
   `-- personal
       `-- dotfiles
   ```

3. Provision Harbour

   ```sh
   harbour provision
   ```

   If you are not using Homebrew, install `colima` before provisioning.

   The first run creates Harbour's local config automatically.

   Provision prompts for:

   - `workspace_path`
   - `harness_path`
   - Agent to provision
   - The default `harbour` command

4. Run the agent

```sh
harbour
```

Or run a command explicitly:

```sh
harbour agent
harbour yolo
harbour shell
```

## Commands

```sh
harbour help
harbour version
harbour provision
harbour shell
harbour agent
harbour yolo
```

## Config

Harbour stores its config as a single JSON file.

```json
{
  "vm_backend": "colima",
  "vm_profile": "harbour",
  "vm_runtime": "docker",
  "vm_type": "vz",
  "vm_arch": "aarch64",
  "vm_cpu": 4,
  "vm_memory": 8,
  "vm_disk": 100,
  "vm_mount_type": "virtiofs",
  "vm_forward_ssh_agent": true,
  "vm_network_address": false,
  "vm_colima_home": "",
  "codex_version": "latest",
  "claude_code_version": "latest",
  "harness_path": "",
  "workspace_path": "",
  "active_agent": "",
  "default_command": "agent"
}
```

### Isolated Colima home and host ports

`vm_colima_home` is the `COLIMA_HOME` for Harbour's VM. Empty (the default) means `~/.harbour/colima` — a home of its own, deliberately **not** the engineer's `~/.colima`.

Running in its own home lets Harbour write a lima override (`$COLIMA_HOME/_lima/_config/override.yaml`) that disables host port forwarding for **its** VM only, so the agent VM never claims host ports the engineer's default Colima VM also publishes (e.g. a dev DB on `127.0.0.1:1119`). The default VM is untouched.

Two things to be aware of:

- **One-time migration.** A `harbour` instance created under the default `~/.colima` before this change is orphaned, and keeps claiming host ports until removed. Run once, with no `COLIMA_HOME` set so it targets the default home: `colima stop -p harbour && colima delete -p harbour`. Harbour then creates a fresh instance in the isolated home.
- **Visibility.** The isolated instance won't appear in a plain `colima list` / `docker context ls`. Manage it with `COLIMA_HOME=~/.harbour/colima` set, or via Harbour. (Harbour applies `COLIMA_HOME` to every colima call it makes, so its own commands always target the right home.)

## Development

```sh
make build
./bin/harbour help
```

`make build` builds a local macOS ARM64 binary for development use.

```sh
go test ./...
```

## Releasing

```sh
make dist VERSION=v0.1.0
```

This writes Homebrew-ready Darwin artefacts to `dist/`:

- `harbour-v0.1.0-darwin-amd64.tar.gz`
- `harbour-v0.1.0-darwin-arm64.tar.gz`
- `sha256sums.txt`

`make dist` verifies the requested tag on `origin`, clones that tag into a temporary release source checkout under `build/`, and builds the release artefacts from that remote tagged source.

Release builds inject the requested version into `harbour version`.

`VERSION` must match `vX.Y.Z`.
