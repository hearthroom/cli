#!/bin/sh
# Build and deploy cli.hearthroom.club (and its sukisuki aliases) from this
# machine with the logged-in wrangler.
#
#   sh site/scripts/deploy.sh            # from the repository root
#   sh scripts/deploy.sh                 # from site/
#
# Steps: generate the manual from the current source with the release tag to
# show, build the static site (Markdown twins, llms.txt, sitemap), run the
# site tests, then `wrangler deploy`. Pass --dry-run to stop before deploying.
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
root=$(cd "$here/.." && pwd)
cd "$root"
git diff --quiet -- site cmd/gendocs internal || { echo "deploy: commit your site changes first (working tree is dirty)" >&2; exit 1; }
version=${CLI_VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0)}
echo "· manual from source at $(git rev-parse --short HEAD), shown as $version"
go run ./cmd/gendocs -out site/src/content/manual -version "$version"
cp docs/card-folder.md site/src/content/guides/card-folder.md
cd site
CLI_VERSION="$version" npx astro build
npx vitest run
if [ "${1:-}" = "--dry-run" ]; then npx wrangler deploy --dry-run; exit 0; fi
npx wrangler deploy
for h in cli.hearthroom.club cli.sukisuki.ai cli.sukisuki.chat; do
  printf '%-22s / %s  /llms.txt %s  /manual/card/push.md %s\n' "$h" \
    "$(curl -sS -o /dev/null -w '%{http_code}' "https://$h/")" \
    "$(curl -sS -o /dev/null -w '%{http_code}' "https://$h/llms.txt")" \
    "$(curl -sS -o /dev/null -w '%{http_code}' "https://$h/manual/card/push.md")"
done
