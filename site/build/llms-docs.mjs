// One source, two readers. People get the rendered HTML; agents get the same
// Markdown at the same path with `.md`, plus /llms.txt (an index) and
// /llms-full.txt (everything English, concatenated). This runs after the
// Astro build and writes straight into dist/, mirroring what hearthroom.club
// does for its own documentation.
import { promises as fs } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const SITE = "https://cli.hearthroom.club";

/** Strip YAML front matter; return { meta, body }. */
export function splitFrontMatter(raw) {
  const m = /^---\n([\s\S]*?)\n---\n?/.exec(raw);
  if (!m) return { meta: {}, body: raw };
  const meta = {};
  for (const line of m[1].split("\n")) {
    const i = line.indexOf(":");
    if (i < 0) continue;
    let v = line.slice(i + 1).trim();
    if (v.startsWith('"') && v.endsWith('"')) v = JSON.parse(v);
    meta[line.slice(0, i).trim()] = v;
  }
  return { meta, body: raw.slice(m[0].length).replace(/^\n+/, "") };
}

/** The pages the twins and llms files describe. Order is the reading order. */
export async function collect(root = path.join(here, "..")) {
  const pages = [];
  const landing = await fs.readFile(path.join(root, "src/content/landing/en.md"), "utf8");
  pages.push({ route: "/", md: "/index.md", title: "Hearthroom CLI", body: splitFrontMatter(landing).body, kind: "landing" });

  const guidesDir = path.join(root, "src/content/guides");
  const order = ["install.md", "card-folder.md", "trial-cards.md", "agents.md"];
  const guideFiles = (await fs.readdir(guidesDir)).filter((n) => n.endsWith(".md")).sort((a, b) => (order.indexOf(a) + 1 || 99) - (order.indexOf(b) + 1 || 99));
  for (const f of guideFiles) {
    const raw = await fs.readFile(path.join(guidesDir, f), "utf8");
    const { meta, body } = splitFrontMatter(raw);
    const slug = f.replace(/\.md$/, "");
    pages.push({ route: `/guides/${slug}/`, md: `/guides/${slug}.md`, title: meta.title || firstHeading(body) || slug, body, kind: "guide" });
  }

  const manualDir = path.join(root, "src/content/manual");
  const index = JSON.parse(await fs.readFile(path.join(manualDir, "index.json"), "utf8"))
    // The root page first, then command groups in alphabetical order.
    .sort((a, b) => (a.slug === "hearthroom" ? -1 : b.slug === "hearthroom" ? 1 : a.slug.localeCompare(b.slug)));
  for (const p of index) {
    const file = path.join(manualDir, p.slug.replaceAll("/", "__") + ".md");
    const { body } = splitFrontMatter(await fs.readFile(file, "utf8"));
    const route = p.slug === "hearthroom" ? "/manual/" : `/manual/${p.slug}/`;
    const md = p.slug === "hearthroom" ? "/manual/index.md" : `/manual/${p.slug}.md`;
    pages.push({ route, md, title: p.command, short: p.short, body, kind: "manual" });
  }
  return pages;
}

function firstHeading(body) {
  const m = /^#\s+(.+)$/m.exec(body);
  return m ? m[1].trim() : "";
}

export function llmsIndex(pages) {
  const lines = [
    "# Hearthroom CLI",
    "",
    "> `hearthroom` is the command-line client for Hearthroom, the open community for AI character cards. Cards are folders of plain files; the CLI pushes them to the connected card provider, validates them, plays them, imports cards from other tools and browses the community. Every command supports --json. Sending a message with `play` spends credits and requires --allow-spend.",
    "",
    "Every page below is also available as Markdown at the same path with a `.md` suffix. The complete text is in /llms-full.txt.",
    "",
    "## Start here",
    "",
  ];
  for (const p of pages.filter((p) => p.kind !== "manual")) {
    lines.push(`- [${p.title}](${SITE}${p.md})`);
  }
  lines.push("", "## Manual (generated from the binary's --help)", "");
  for (const p of pages.filter((p) => p.kind === "manual")) {
    lines.push(`- [${p.title}](${SITE}${p.md})${p.short ? `: ${p.short}` : ""}`);
  }
  lines.push("", "## Source", "", `- [GitHub](https://github.com/hearthroom/cli)`, `- [Releases](https://github.com/hearthroom/cli/releases)`, "");
  return lines.join("\n");
}

export function llmsFull(pages) {
  const parts = ["# Hearthroom CLI — complete documentation", "", `Canonical site: ${SITE}. Each section is also served at the URL shown.`, ""];
  for (const p of pages) {
    parts.push(`<!-- ${SITE}${p.md} -->`, "", p.body.trim(), "", "---", "");
  }
  return parts.join("\n");
}

async function writeTwins(dist, pages) {
  for (const p of pages) {
    const target = path.join(dist, p.md.replace(/^\//, ""));
    await fs.mkdir(path.dirname(target), { recursive: true });
    await fs.writeFile(target, p.body.trim() + "\n");
  }
  await fs.writeFile(path.join(dist, "llms.txt"), llmsIndex(pages) + "\n");
  await fs.writeFile(path.join(dist, "llms-full.txt"), llmsFull(pages) + "\n");
}

/** Astro integration. */
export function llmsDocs() {
  return {
    name: "hearthroom-llms-docs",
    hooks: {
      "astro:build:done": async ({ dir, logger }) => {
        const dist = fileURLToPath(dir);
        const pages = await collect();
        await writeTwins(dist, pages);
        logger.info(`wrote ${pages.length} Markdown twins, llms.txt and llms-full.txt`);
      },
    },
  };
}
