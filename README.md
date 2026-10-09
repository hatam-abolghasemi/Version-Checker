# Version Checker

Watches upstream projects for new stable releases and posts them to a Telegram channel.
Runs entirely on GitHub Actions every 5 minutes. No servers needed.

## How it works

1. `sources.txt` lists what to track.
2. `check.sh` resolves the newest stable version of each source.
3. Anything newer than `state.json` is fetched again to confirm, then sent to Telegram.
4. The workflow commits the updated `state.json` back to the repo.

New sources are recorded silently on their first run. Downgrades are ignored.
If a source can't be resolved, a warning is posted to the channel.

## Source types

| Type             | Target         | Method                                         |
|------------------|----------------|------------------------------------------------|
| `github-release` | `owner/repo`   | GitHub Releases API, skips drafts/prereleases  |
| `github-tag`     | `owner/repo`   | `git ls-remote --tags`                         |
| `url`            | any URL        | PCRE regex on the page (regex required)        |

The optional 4th column is a regex the tag must match.
Default: `^v?[0-9]+(\.[0-9]+)+$` (stable semver only).

```
kubernetes   github-release  kubernetes/kubernetes
zookeeper    github-tag      apache/zookeeper   ^release-[0-9]+\.[0-9]+\.[0-9]+$
haproxy      url             https://www.haproxy.org/   haproxy-\K[0-9]+\.[0-9]+\.[0-9]+(?=\.tar\.gz)
```

## Setup

1. Create a Telegram bot with @BotFather and add it as an admin of your channel.
2. Add repository secrets:
   - `TELEGRAM_BOT_TOKEN`
   - `TELEGRAM_CHAT_ID` (`@channelname` for public channels, `-100…` for private)
3. Run the workflow once from the Actions tab to seed `state.json`.

If the state commit fails with 403, set **Settings → Actions → General → Workflow permissions** to **Read and write**.

## Local testing

```bash
DRY_RUN=1 bash check.sh
```

Prints messages instead of sending them. Requires `curl`, `jq`, `git`.
