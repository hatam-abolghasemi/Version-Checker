# Changelog

## 1.0.1

Fewer false alarms, no stuck or lost messages, and a fraction of the
download per run.

### Changed

- **Runs every 15 minutes** instead of every 5: 96 runner jobs a day
  instead of 288.
- **A source is reported as failing after 3 failed runs in a row**
  (`-fail-threshold`), not on the first error. A one-off network blip that
  recovers sooner stays silent.
- **`github-release` uses the GraphQL API** when a token is set, fetching
  only tag, URL and draft/prerelease flags: a few KB instead of every
  release's changelog body, which is much larger for big projects. Without
  a token it falls back to REST, now decoded one release at a time.
- **The `check` workflow runs a pinned release** (`VERSION_CHECKER_VERSION`)
  and verifies it against `checksums.txt`. A new tag no longer reaches the
  schedule by itself; the release workflow fails if the pin and the tag
  disagree.
- **Releases follow a template:** `vX.Y.Z — title` from the annotated tag,
  notes from this file.

### Fixed

- **Messages over Telegram's 4096-character limit** were rejected, and the
  run retried the same message forever. They are now split into several,
  and state is saved after each one, so a partial send is never repeated.
- **Telegram retries** only 429 and 5xx responses and waits for
  `retry_after`; a 400 (bad HTML, wrong chat) fails at once.
- **A "Recovered" message could be lost** when sending it failed. The
  failing mark is now cleared only once it is delivered.
- **Pre-releases sorted above their release** (`1.10.0-rc1` > `1.10.0`)
  with a pattern that lets them through. `9.3-1` still sorts above `9.3`.
- **State is saved even when a send fails**, and the workflow commits it,
  so what was delivered is recorded.

## 1.0.0

First release.
