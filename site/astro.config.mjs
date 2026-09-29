import { defineConfig } from "astro/config";
import { llmsDocs } from "./build/llms-docs.mjs";
import { latestVersion } from "./build/version.mjs";
import sitemap from "@astrojs/sitemap";

const version = await latestVersion();

export default defineConfig({
  site: "https://cli.hearthroom.club",
  output: "static",
  trailingSlash: "always",
  build: { format: "directory" },
  i18n: {
    defaultLocale: "en",
    locales: ["en", "zh-Hant", "zh-Hans", "ja", "ko"],
    routing: { prefixDefaultLocale: false },
  },
  markdown: { smartypants: false },
  vite: { define: { __CLI_VERSION__: JSON.stringify(version) } },
  integrations: [sitemap(), llmsDocs()],
});
