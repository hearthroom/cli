import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const routes = readFileSync(new URL('../wrangler.toml', import.meta.url), 'utf8');
const verifier = readFileSync(new URL('../scripts/verify-hosts.mjs', import.meta.url), 'utf8');

describe('CLI official hosts', () => {
  it('deploys and reads back all four host routes', () => {
    for (const root of ['hearthroom.club', 'sukisuki.ai', 'sukisuki.chat', 'sukisuki.pro']) {
      expect(routes).toContain(`pattern = "cli.${root}/*", zone_name = "${root}"`);
      expect(verifier).toContain(`cli.${root}`);
    }
  });
});
