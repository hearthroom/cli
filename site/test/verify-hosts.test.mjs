import { expect, it } from 'vitest';
import { verifyHosts } from '../scripts/verify-hosts.mjs';

it('rejects a successful HTTP response from the community site', async () => {
  const fetcher = async (url) => new Response(url.includes('sukisuki.pro') ? '# Hearthroom\n' : '# Hearthroom CLI\nidentical', { status: 200 });
  await expect(verifyHosts(['cli.sukisuki.ai', 'cli.sukisuki.pro'], fetcher)).rejects.toThrow('CLI content');
});

it('requires every host to serve identical CLI content', async () => {
  const fetcher = async (url) => new Response('# Hearthroom CLI\n' + (url.includes('sukisuki.pro') ? 'different' : 'same'));
  await expect(verifyHosts(['cli.sukisuki.ai', 'cli.sukisuki.pro'], fetcher)).rejects.toThrow('differs');
  await expect(verifyHosts(['cli.sukisuki.ai', 'cli.sukisuki.chat'], fetcher)).resolves.toBeUndefined();
});

it('rejects matching old content when it differs from the current build', async () => {
  const fetcher = async () => new Response('# Hearthroom CLI\nold');
  await expect(verifyHosts(['cli.sukisuki.ai', 'cli.sukisuki.pro'], fetcher, '# Hearthroom CLI\nnew')).rejects.toThrow('build');
});
