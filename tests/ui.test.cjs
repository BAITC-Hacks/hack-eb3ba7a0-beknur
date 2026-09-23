const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');

test('UI initializes, rejects unknown districts atomically, and discards stale AI replies', async () => {
  const elements = new Map();
  let tool, resolveFetch;
  const document = {
    querySelector(id) {
      if (!elements.has(id)) elements.set(id, { style:{}, handlers:{}, textContent:'', addEventListener(name, fn) { this.handlers[name] = fn; } });
      return elements.get(id);
    },
    querySelectorAll: () => [],
    modelContext: { registerTool(value) { tool = value; } },
  };
  const ctx = vm.createContext({ document, location:{protocol:'http:'}, AbortController, setTimeout, clearTimeout,
    fetch: () => new Promise(resolve => { resolveFetch = resolve; }),
  });
  const read = name => fs.readFileSync(path.join(__dirname, '../dist', name), 'utf8');
  vm.runInContext(read('city-model.js'), ctx);
  vm.runInContext(read('ai-ui.js'), ctx);
  vm.runInContext(read('index.html').match(/<script>([\s\S]*?)<\/script>/)[1], ctx);
  const button = elements.get('#ai-analyze');
  assert.equal(button.disabled, true);
  const decisions = [{id:'M7',district:'Нура'},{id:'M8',district:'Нура'},{id:'M10',district:'Нура'},{id:'M12'},{id:'M5',district:'Сарыарка'}];
  assert.equal(tool.execute({decisions}).score, 56.54);
  assert.equal(button.disabled, false);
  const before = vm.runInContext('JSON.stringify(state)', ctx);
  assert.throws(() => tool.execute({decisions:[{id:'M7',district:'Unknown'},...decisions.slice(1)]}));
  assert.equal(vm.runInContext('JSON.stringify(state)', ctx), before);
  const request = button.handlers.click();
  assert.equal(button.disabled, true);
  elements.get('#reset').handlers.click();
  resolveFetch(Response.json({analysis:'stale reply'}));
  await request;
  assert.equal(elements.get('#ai-result').textContent, '');
  assert.equal(button.disabled, true);
  tool.execute({decisions});
  const next = button.handlers.click();
  resolveFetch(Response.json({analysis:'<script>plain text</script>'}));
  await next;
  assert.equal(elements.get('#ai-result').textContent, '<script>plain text</script>');
  assert.equal(button.disabled, false);
});
