// Parses every mermaid block in the docs. Zensical renders them in the browser, so
// `zensical build --strict` never looks at them and a syntax error just means a diagram
// that does not draw.
//
// Pinned to the same major version the site loads, since keywords differ between them.
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { JSDOM } from 'jsdom';

const docs = process.argv[2] ?? 'docs';

// Mermaid needs a DOM, and it has to be in place before the import, because DOMPurify
// grabs the window when its module loads.
const dom = new JSDOM('<!doctype html><html><body></body></html>', {
  pretendToBeVisual: true,
  url: 'http://localhost/',
});
for (const name of [
  'window', 'document', 'Element', 'SVGElement', 'HTMLElement',
  'DOMParser', 'Node', 'NodeFilter', 'getComputedStyle',
]) {
  globalThis[name] = dom.window[name] ?? dom.window;
}
// navigator is getter-only on newer Node, so assigning it throws.
Object.defineProperty(globalThis, 'navigator', {
  value: dom.window.navigator,
  configurable: true,
});

const mermaid = (await import('mermaid')).default;
mermaid.initialize({ startOnLoad: false });

function markdownFiles(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      out.push(...markdownFiles(path));
    } else if (entry.endsWith('.md')) {
      out.push(path);
    }
  }
  return out.sort();
}

// Returns each block with the line it starts on, for the error message.
function blocks(path) {
  const found = [];
  const lines = readFileSync(path, 'utf8').split('\n');
  let start = -1;
  let body = [];
  for (let i = 0; i < lines.length; i++) {
    if (start < 0 && lines[i].trimEnd() === '```mermaid') {
      start = i + 1;
      body = [];
      continue;
    }
    if (start >= 0) {
      if (lines[i].trimEnd() === '```') {
        found.push({ line: start, text: body.join('\n') });
        start = -1;
        continue;
      }
      body.push(lines[i]);
    }
  }
  return found;
}

let total = 0;
let failures = 0;
for (const path of markdownFiles(docs)) {
  for (const block of blocks(path)) {
    total++;
    const where = `${relative('.', path)}:${block.line}`;
    try {
      await mermaid.parse(block.text);
    } catch (err) {
      failures++;
      const message = String(err?.message ?? err).replace(/\s+/g, ' ').trim();
      console.error(`${where}: ${message}`);
    }
  }
}

console.log(`${total} mermaid blocks, ${failures} with syntax errors`);
process.exit(failures ? 1 : 0);
