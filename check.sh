#!/usr/bin/env bash
set -uo pipefail

SOURCES_FILE="${SOURCES_FILE:-sources.txt}"
STATE_FILE="${STATE_FILE:-state.json}"
DEFAULT_REGEX='^v?[0-9]+(\.[0-9]+)+$'
DRY_RUN="${DRY_RUN:-0}"
export GIT_TERMINAL_PROMPT=0

for bin in curl jq git; do
  command -v "$bin" >/dev/null || { echo "missing dependency: $bin" >&2; exit 1; }
done

if [[ "$DRY_RUN" != 1 ]]; then
  : "${TELEGRAM_BOT_TOKEN:?TELEGRAM_BOT_TOKEN is required}"
  : "${TELEGRAM_CHAT_ID:?TELEGRAM_CHAT_ID is required}"
fi

gh_auth=()
[[ -n "${GITHUB_TOKEN:-}" ]] && gh_auth=(-H "Authorization: Bearer ${GITHUB_TOKEN}")

newest() {
  grep -E -- "$1" | sed -E 's/^[^0-9]*//' | sort -V | tail -n1
}

fetch_github_release() {
  curl -fsSL --retry 3 --max-time 30 \
    -H "Accept: application/vnd.github+json" "${gh_auth[@]}" \
    "https://api.github.com/repos/$1/releases?per_page=100" |
    jq -r '.[] | select(.draft | not) | select(.prerelease | not) | .tag_name' |
    newest "$2"
}

fetch_github_tag() {
  git ls-remote --tags --refs "https://github.com/$1.git" |
    sed 's#.*refs/tags/##' |
    newest "$2"
}

fetch_url() {
  curl -fsSL --retry 3 --max-time 30 -A "Mozilla/5.0" "$1" |
    grep -oP -- "$2" | sort -V | tail -n1
}

send_telegram() {
  if [[ "$DRY_RUN" == 1 ]]; then
    printf -- '--- telegram ---\n%s\n----------------\n' "$1"
    return 0
  fi
  curl -fsS --retry 3 --max-time 30 -X POST \
    "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=${TELEGRAM_CHAT_ID}" \
    --data-urlencode "text=$1" \
    --data-urlencode "parse_mode=HTML" \
    --data-urlencode "disable_web_page_preview=true" >/dev/null
}

[[ -s "$STATE_FILE" ]] || echo '{}' > "$STATE_FILE"
state=$(cat "$STATE_FILE")
updates=()
failures=()

fetch() {
  case "$1" in
    github-release) fetch_github_release "$2" "${3:-$DEFAULT_REGEX}" ;;
    github-tag)     fetch_github_tag "$2" "${3:-$DEFAULT_REGEX}" ;;
    url)            [[ -n "${3:-}" ]] && fetch_url "$2" "$3" ;;
  esac
}

while read -r -u 3 name type target regex; do
  [[ -z "${name:-}" || "$name" == \#* ]] && continue
  case "$type" in
    github-release) link="https://github.com/$target/releases" ;;
    github-tag)     link="https://github.com/$target/tags" ;;
    url)            link="$target" ;;
    *)
      failures+=("$name (unknown type: $type)")
      continue ;;
  esac
  version=$(fetch "$type" "$target" "${regex:-}")

  if [[ -z "$version" ]]; then
    echo "FAIL  $name" >&2
    failures+=("$name")
    continue
  fi

  old=$(jq -r --arg n "$name" '.[$n] // ""' <<<"$state")
  echo "OK    $name ${old:-<new>} -> $version" >&2

  if [[ -z "$old" ]]; then
    state=$(jq --arg n "$name" --arg v "$version" '.[$n] = $v' <<<"$state")
  elif [[ "$old" != "$version" && "$(printf '%s\n%s\n' "$old" "$version" | sort -V | tail -n1)" == "$version" ]]; then
    sleep "${VERIFY_DELAY:-20}"
    confirm=$(fetch "$type" "$target" "${regex:-}")
    if [[ "$confirm" != "$version" ]]; then
      echo "SKIP  $name $version not confirmed (got ${confirm:-nothing}), retrying next run" >&2
      continue
    fi
    updates+=("<b>${name}</b>: ${old} → <a href=\"${link//&/&amp;}\">${version}</a>")
    state=$(jq --arg n "$name" --arg v "$version" '.[$n] = $v' <<<"$state")
  fi
done 3< "$SOURCES_FILE"

if (( ${#updates[@]} )); then
  msg="🆕 <b>New versions</b>"$'\n\n'"$(printf '%s\n' "${updates[@]}")"
  send_telegram "$msg" || { echo "telegram send failed, state not saved" >&2; exit 1; }
fi

if (( ${#failures[@]} )); then
  send_telegram "⚠️ version-checker could not resolve: $(IFS=,; echo "${failures[*]}")" || true
fi

jq -S . <<<"$state" > "$STATE_FILE"
