import { describe, it, expect } from "vitest";
import { collect, llmsIndex, llmsFull, splitFrontMatter } from "../build/llms-docs.mjs";

describe("Markdown twins and llms files", () => {
  it("strips front matter and keeps the body", () => {
    const { meta, body } = splitFrontMatter('---\ntitle: "X"\nslug: "a/b"\n---\n\n# X\n\nbody\n');
    expect(meta).toEqual({ title: "X", slug: "a/b" });
    expect(body).toBe("# X\n\nbody\n");
  });

  it("gives every page a twin at the same path with .md and lists all of them", async () => {
    const pages = await collect();
    const routes = pages.map((p) => p.route);
    expect(routes).toContain("/");
    expect(routes).toContain("/manual/");
    expect(routes).toContain("/manual/card/push/");
    expect(routes).toContain("/guides/card-folder/");
    for (const p of pages) {
      // twin path = route with trailing slash replaced by .md (index.md for section roots)
      const expected = p.route === "/" ? "/index.md" : p.route === "/manual/" ? "/manual/index.md" : p.route.replace(/\/$/, ".md");
      expect(p.md).toBe(expected);
      expect(p.body.trim().length).toBeGreaterThan(0);
      expect(p.body).not.toMatch(/^---\n/);
    }
    const index = llmsIndex(pages);
    for (const p of pages) expect(index).toContain(`https://cli.hearthroom.club${p.md}`);
    expect(index.startsWith("# Hearthroom CLI\n\n> ")).toBe(true);
  });

  it("concatenates the manual and guides into llms-full.txt in reading order", async () => {
    const pages = await collect();
    const full = llmsFull(pages);
    const at = (md) => full.indexOf(`<!-- https://cli.hearthroom.club${md} -->`);
    expect(at("/index.md")).toBeGreaterThan(-1);
    expect(at("/index.md")).toBeLessThan(at("/guides/install.md"));
    expect(at("/guides/agents.md")).toBeLessThan(at("/manual/index.md"));
    expect(at("/manual/index.md")).toBeLessThan(at("/manual/card/push.md"));
    expect(full).toContain("# hearthroom card push");
    expect(full).toContain("--allow-spend");
  });

  it("manual pages come from the generated index and carry the command name", async () => {
    const pages = (await collect()).filter((p) => p.kind === "manual");
    expect(pages.length).toBeGreaterThan(20);
    for (const p of pages) {
      expect(p.title.startsWith("hearthroom")).toBe(true);
      expect(p.body).toContain(`# ${p.title}`);
    }
  });
});
