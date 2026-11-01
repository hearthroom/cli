import { pathToFileURL } from 'node:url';
import { readFile } from 'node:fs/promises';

export const HOSTS = ['cli.hearthroom.club', 'cli.sukisuki.ai', 'cli.sukisuki.chat', 'cli.sukisuki.pro'];

export async function verifyHosts(hosts = HOSTS, fetcher = fetch, buildContent) {
  let expected;
  for (const host of hosts) {
    const response = await fetcher(`https://${host}/llms.txt`, { redirect: 'error', signal: AbortSignal.timeout(20000) });
    if (!response.ok) throw new Error(`${host}: HTTP ${response.status}`);
    const body = await response.text();
    if (!body.startsWith('# Hearthroom CLI\n')) throw new Error(`${host}: not CLI content`);
    if (buildContent !== undefined && body !== buildContent) throw new Error(`${host}: content differs from build`);
    if (expected !== undefined && body !== expected) throw new Error(`${host}: content differs`);
    expected = body;
    console.log(`${host}: CLI llms.txt verified`);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  readFile(new URL('../dist/llms.txt', import.meta.url), 'utf8')
    .then((built) => verifyHosts(HOSTS, fetch, built))
    .catch((error) => { console.error(error.message); process.exitCode = 1; });
}
