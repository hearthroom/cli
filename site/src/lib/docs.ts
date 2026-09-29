import type { MarkdownInstance } from "astro";

export type ManualEntry = { slug: string; command: string; short: string; parent?: string; children?: string[] };
export const manualIndex: ManualEntry[] = (await import("../content/manual/index.json")).default;

const manualFiles = import.meta.glob<MarkdownInstance<{ slug: string; command: string; short: string; parent?: string }>>("../content/manual/*.md", { eager: true });
export const manualPages = Object.values(manualFiles).map((m) => m);
export function manualBySlug(slug: string) {
  return manualPages.find((m) => m.frontmatter.slug === slug);
}

const guideFiles = import.meta.glob<MarkdownInstance<{ title: string }>>("../content/guides/*.md", { eager: true });
export const guides = Object.entries(guideFiles).map(([file, mod]) => ({ slug: file.split("/").pop()!.replace(/\.md$/, ""), mod }));
export const guideOrder = ["install", "card-folder", "agents"];
export const guidesSorted = [...guides].sort((a, b) => guideOrder.indexOf(a.slug) - guideOrder.indexOf(b.slug));
