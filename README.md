# Version Checker

Watches upstream projects for new stable releases and posts them to a Telegram channel.
Runs on GitHub Actions every 15 minutes. No servers needed.

Each new version in a message links to where it was found: the GitHub release page
(with its changelog), the tag page, or the source webpage with the version highlighted.

## How it works

1. `tools.yaml` lists what to track.
2. Every source is fetched in parallel and the newest version matching its pattern is picked.
3. Anything newer than `state.json` is fetched again after a short delay to confirm it.
4. Confirmed updates are sent to Telegram, then `state.json` is saved and committed.

- New tools are recorded silently on their first run.
- Downgrades are ignored.
- If sending an update fails, its state is not advanced and the next run retries it.
- A source is reported as failing after 3 failed runs in a row (`-fail-threshold`), once, and again when it recovers.
  A one-off blip that recovers sooner stays silent.
- A message over Telegram's 4096-character limit is split into several. Progress is saved after each one,
  so a partial send never repeats what was already delivered.
- Telegram 429 and 5xx responses are retried (honoring `retry_after`); other errors are not.
- Tools removed from `tools.yaml` are dropped from `state.json` automatically.

## Configuration

```yaml
defaults:
  pattern: '^v?[0-9]+(\.[0-9]+)+$'

telegram:
  template: ""

tools:
  - name: argocd
    display_name: Argo CD
    description: GitOps continuous delivery
    homepage: https://argo-cd.readthedocs.io
    labels: [gitops, k8s]
    meta:
      owner: platform
    source:
      type: github-release
      repo: argoproj/argo-cd
      pattern: '^v[0-9]+\.[0-9]+\.[0-9]+$'
```

| Field                | Required | Notes                                                       |
|----------------------|----------|-------------------------------------------------------------|
| `name`               | yes      | Unique key, also used in `state.json`                       |
| `display_name`       | no       | Shown in messages instead of `name`                         |
| `description`        | no       | Free text, available to the template                        |
| `homepage`           | no       | Available to the template                                   |
| `labels`             | no       | List of strings, available to the template                  |
| `meta`               | no       | Free-form string map for anything else                      |
| `source.type`        | yes      | `github-release`, `github-tag` or `webpage`                 |
| `source.repo`        | github-* | `owner/repo`                                                |
| `source.url`         | webpage  | Page to scrape                                              |
| `source.pattern`     | webpage  | Go regexp. For `webpage` it needs exactly one capture group |

Unknown fields are rejected, so typos fail fast.

### Source types

| Type             | How it fetches                                                       | Link in message            |
|------------------|----------------------------------------------------------------------|----------------------------|
| `github-release` | GitHub GraphQL API (REST without a token), skips drafts and prereleases | The release page        |
| `github-tag`     | Git smart-HTTP ref list (no API quota used)                          | The tag page               |
| `webpage`        | Downloads the page and takes capture group 1 of every pattern match | The page, version highlighted |

For `github-*` sources, `pattern` filters tag names (default: stable semver with an optional `v`).
The version is the tag with any leading non-digits removed, so `release-3.9.6` becomes `3.9.6`.

### Message template

`telegram.template` overrides the default message. It's a Go `html/template` rendered with Telegram HTML.
The data is:

```
.Updates    []{ .Tool, .Old, .New{ .Version, .Tag, .URL } }
.Failures   []{ .Tool, .Error }
.Recovered  []Tool
```

`.Tool` exposes every config field (`.Tool.Title`, `.Tool.Homepage`, `.Tool.Labels`, `.Tool.Meta.owner`, …).
See `DefaultTemplate` in `internal/notify/notify.go` for the starting point.

## Setup

1. Create a Telegram bot with @BotFather and add it as an admin of your channel.
2. Add repository secrets `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID`
   (`@channelname` for public channels, `-100…` for private ones).
3. Release: add a `## X.Y.Z` section to `CHANGELOG.md`, set `VERSION_CHECKER_VERSION: vX.Y.Z` in
   `.github/workflows/check.yml`, then push an annotated tag. Its message becomes the title after the dash:
   `git tag -a vX.Y.Z -m "short description" && git push origin vX.Y.Z`.
4. Run the `check` workflow once from the Actions tab.

The `check` workflow runs the release named in `VERSION_CHECKER_VERSION` at the top of `.github/workflows/check.yml`
and verifies its checksum. Code changes take effect once you tag a release *and* bump that value.
Changes to `tools.yaml` take effect immediately.

If the state commit fails with 403, set **Settings → Actions → General → Workflow permissions** to **Read and write**.

## Local usage

```bash
go run ./cmd/version-checker -dry-run
```

| Flag            | Default       | Notes                                        |
|-----------------|---------------|----------------------------------------------|
| `-config`       | `tools.yaml`  |                                              |
| `-state`        | `state.json`  |                                              |
| `-dry-run`      | `false`       | Print messages instead of sending (or `DRY_RUN=1`) |
| `-verify-delay` | `20s`         | Wait before the confirming fetch             |
| `-parallelism`  | `6`           | Sources fetched at once                      |
| `-fail-threshold` | `3`         | Consecutive failed runs before a "Failing" alert |
| `-timeout`      | `4m`          | Overall run timeout                          |
| `-version`      |               | Print version and exit                       |

Environment: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `GITHUB_TOKEN` (optional locally; with it `github-release`
uses GraphQL, which fetches a few KB instead of every changelog, and the rate limit is higher).

## Layout

```
cmd/version-checker   entrypoint and flags
internal/config       tools.yaml schema and validation
internal/source       Source interface and github-release, github-tag, webpage implementations
internal/version      version normalization and natural comparison
internal/state        state.json (reads the old flat format too)
internal/notify       Notifier interface, message template, Telegram and stdout
internal/checker      fetch, compare, verify, notify, failure tracking
```

### Adding a source type

1. Implement `source.Source` (`Latest(ctx) (Release, error)`) in `internal/source`.
2. Register it in `source.New` and add the type constant and validation in `internal/config`.

### Adding a notifier

Implement `notify.Notifier` (`Notify(ctx, Report) error`) and wire it up in `cmd/version-checker/main.go`.
If it has a message size limit, also implement `notify.Splitter` (`Split(Report) ([]Report, error)`);
`Renderer.Split(rep, limit)` does the packing.
