#!/usr/bin/env bash
#
# Start an ad-hoc NodePing maintenance window.
#
# Ad-hoc maintenance cannot be modelled as an OpenTofu resource: NodePing
# auto-deletes the record once the window completes, so a declarative resource
# would see it vanish, drop it from state, and recreate it on the next plan --
# an infinite loop that repeatedly disables monitoring. See docs/assumptions.md
# A-003.
#
# This script is the imperative escape hatch, intended to be driven from a
# `terraform_data` resource with a `local-exec` provisioner. See
# docs/resources/nodeping_maintenance.md for the recipe.
#
# The API token is read from the environment, never from an argument, so it
# does not appear in `ps` output or shell history.
#
# Required environment:
#   NODEPING_API_TOKEN    NodePing API token
#
# Optional environment:
#   NODEPING_CUSTOMER_ID  Subaccount to scope the window to
#   NODEPING_API_URL      API base URL (default https://api.nodeping.com/api/1)
#
# Usage:
#   nodeping-adhoc-maintenance.sh --duration 30 --checks ID1,ID2 [--name "..."]

set -euo pipefail

readonly DEFAULT_API_URL="https://api.nodeping.com/api/1"
readonly MAX_DURATION=527040 # ~1 year, matching the resource's bound
readonly MAX_CHECKS=5000
readonly HTTP_TIMEOUT=30

die() {
	printf 'nodeping-adhoc-maintenance: %s\n' "$1" >&2
	exit 1
}

usage() {
	sed -n '3,28p' "$0" | sed 's/^# \{0,1\}//'
	exit "${1:-0}"
}

duration=""
checks=""
name="adhoc-$(date -u +%Y%m%dT%H%M%SZ)"

while [ $# -gt 0 ]; do
	case "$1" in
	--duration)
		duration="${2:-}"
		shift 2
		;;
	--checks)
		checks="${2:-}"
		shift 2
		;;
	--name)
		name="${2:-}"
		shift 2
		;;
	-h | --help) usage 0 ;;
	*) die "unknown argument: $1 (try --help)" ;;
	esac
done

# --- validate inputs before touching the network ---

[ -n "${NODEPING_API_TOKEN:-}" ] || die "NODEPING_API_TOKEN is not set"
[ -n "$duration" ] || die "--duration is required"
[ -n "$checks" ] || die "--checks is required"

case "$duration" in
'' | *[!0-9]*) die "--duration must be a positive integer (minutes), got: $duration" ;;
esac
[ "$duration" -ge 1 ] || die "--duration must be at least 1 minute"
[ "$duration" -le "$MAX_DURATION" ] ||
	die "--duration $duration exceeds $MAX_DURATION minutes; a value this large is almost
certainly a seconds-or-milliseconds mistake, which would disable monitoring indefinitely"

# Comma-separated check IDs -> JSON array, validating each. NodePing check IDs
# are alphanumeric with hyphens; anything else is rejected rather than escaped,
# so a malformed id cannot alter the request body.
checklist_json="["
count=0
IFS=',' read -ra ids <<<"$checks"
for id in "${ids[@]}"; do
	id="$(printf '%s' "$id" | tr -d '[:space:]')"
	[ -n "$id" ] || continue
	case "$id" in
	*[!A-Za-z0-9-]*) die "invalid check id: $id" ;;
	esac
	[ "$count" -eq 0 ] || checklist_json+=","
	checklist_json+="\"$id\""
	count=$((count + 1))
done
checklist_json+="]"

[ "$count" -ge 1 ] || die "--checks contained no valid check ids"
[ "$count" -le "$MAX_CHECKS" ] || die "--checks exceeds $MAX_CHECKS entries"

case "$name" in
*[!A-Za-z0-9_.:-]*) die "--name may contain only letters, digits and _ . : -" ;;
esac
[ "${#name}" -le 255 ] || die "--name exceeds 255 characters"

api_url="${NODEPING_API_URL:-$DEFAULT_API_URL}"
case "$api_url" in
https://*) ;;
http://localhost* | http://127.0.0.1*) ;;
*) die "NODEPING_API_URL must be https (loopback http is allowed for testing): $api_url" ;;
esac

command -v curl >/dev/null 2>&1 || die "curl is required but not installed"

# --- build and send ---

url="${api_url}/maintenance"
if [ -n "${NODEPING_CUSTOMER_ID:-}" ]; then
	case "$NODEPING_CUSTOMER_ID" in
	*[!A-Za-z0-9-]*) die "invalid NODEPING_CUSTOMER_ID" ;;
	esac
	url="${url}?customerid=${NODEPING_CUSTOMER_ID}"
fi

# id=ad-hoc is what makes this an ad-hoc window rather than a scheduled one.
body=$(printf '{"id":"ad-hoc","name":"%s","duration":%s,"enabled":true,"checklist":%s}' \
	"$name" "$duration" "$checklist_json")

printf 'starting ad-hoc maintenance: %s check(s) for %s minute(s)\n' "$count" "$duration" >&2

# The token reaches curl as a config file on stdin (-K -), never on the command
# line, so it cannot appear in the process list. Backslashes and double quotes
# are escaped for curl's quoted-string syntax. -sS keeps output quiet but still
# reports errors.
token_escaped=${NODEPING_API_TOKEN//\\/\\\\}
token_escaped=${token_escaped//\"/\\\"}
response=$(
	printf 'user = "%s:"\n' "$token_escaped" | curl -sS -K - \
		--max-time "$HTTP_TIMEOUT" \
		-H 'Content-Type: application/json' \
		-w '\n%{http_code}' \
		-X POST \
		-d "$body" \
		"$url"
) || die "request failed (network or timeout after ${HTTP_TIMEOUT}s)"

status="${response##*$'\n'}"
payload="${response%$'\n'*}"

# NodePing returns errors with HTTP 200 on some endpoints, so the status code
# alone is not sufficient -- see docs/assumptions.md A-002.
case "$payload" in
*'"error"'*) die "API returned an error: $payload" ;;
esac

case "$status" in
2*) ;;
*) die "API returned HTTP $status: $payload" ;;
esac

printf 'ad-hoc maintenance started: %s\n' "$payload" >&2
