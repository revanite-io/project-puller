# project-puller

Clone or update all repositories listed in a [Security Insights](https://github.com/ossf/security-insights-spec) file. Reads `project.repositories` from a local file or URL, then runs `git clone` or `git pull` for each repo.

## Install

```bash
go install github.com/revanite-io/project-puller@latest
```

Requires Go 1.25+ and a working `git` on your PATH.

## Usage

**Local file:**

```bash
project-puller /path/to/security-insights.yml
```

**URL:**

```bash
project-puller https://raw.githubusercontent.com/org/repo/main/security-insights.yml
```

**Load the Security Insights file from a GitHub repo** (default path: `security-insights.yml`):

```bash
project-puller --github org/repo
```

Repos are written under the current directory by default. The output directory defaults to the project name from the file when set; override it with `--output`:

```bash
project-puller security-insights.yml --output ./my-repos
```

## Options

| Flag | Short | Description |
|------|-------|-------------|
| `--source` | `-s` | Path or HTTP(S) URL to the Security Insights file (or pass as first argument) |
| `--github` | `-g` | Load from GitHub as `owner/repo` or `owner/repo/path` |
| `--output` | | Directory for cloned repos (default: project name or current dir) |
| `--username` | `-u` | Your fork username: clone with remote `upstream`, add your fork as `origin` |
| `--ssh` | | Use SSH URLs for clone and remotes (default: HTTPS) |
| `--quiet` | `-q` | Suppress git command output |

## Examples

Clone all project repos via HTTPS into `./my-project`:

```bash
project-puller https://example.com/org/project/security-insights.yml --output my-project
```

Same, but use SSH and set up your fork as `origin` with the upstream project as `upstream`:

```bash
project-puller --github org/project --output my-project --username yourname --ssh
```

After that, each repo has `upstream` (the project) and `origin` (your fork); clone and pull use `upstream`.

## Troubleshooting

**Command not found after install**

The Go bin directory may not be on your `PATH`. Add it and confirm:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
which project-puller
project-puller --help
```

**I can't use `go install`**

You can build the binary locally:

```bash
git clone git@github.com:revanite-io/project-puller.git
cd project-puller
go install .
```

Then add the binary `project-puller` to your path, or otherwise run it however you use similar CLI tools.

## Contributing

PRs are welcome. Questions and feature requests can be added via GitHub Issues.
