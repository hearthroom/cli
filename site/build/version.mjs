// Latest release tag, resolved at build time from the releases/latest redirect
// (no GitHub API, no rate limit). Falls back to CLI_VERSION or the last known tag.
export async function latestVersion(fallback = "v0.1.5") {
  if (process.env.CLI_VERSION) return process.env.CLI_VERSION;
  try {
    const res = await fetch("https://github.com/hearthroom/cli/releases/latest", { redirect: "manual" });
    const loc = res.headers.get("location") ?? "";
    const m = /\/tag\/(v[^/]+)$/.exec(loc);
    if (m) return m[1];
  } catch {}
  return fallback;
}
