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
  vm.runInContext(read('optimizer.js'), ctx);
  vm.runInContext(read('optimizer-ui.js'), ctx);
  vm.runInContext(read('city-presets.js'), ctx);
  vm.runInContext(read('config-ui.js'), ctx);
  vm.runInContext(read('app.js'), ctx);
  const button = elements.get('#ai-analyze');
  assert.equal(elements.get('#simulation').hidden, true);
  assert.equal(elements.get('#optimize').disabled, true);
  assert.throws(()=>tool.execute({decisions:[]}));
  elements.get('#configuration-form').handlers.submit({preventDefault(){}});
  assert.match(elements.get('#config-errors').textContent,/бюджет/);
  elements.get('#config-city').value='astana';
  elements.get('#config-example').handlers.click();
  assert.equal(elements.get('#simulation').hidden, true);
  elements.get('#configuration-form').handlers.submit({preventDefault(){}});
  assert.equal(elements.get('#simulation').hidden, false);
  assert.equal(button.disabled, true);
  const decisions = [{id:'M7',district:'Нура'},{id:'M8',district:'Нура'},{id:'M10',district:'Нура'},{id:'M12'},{id:'M5',district:'Сарыарка'}];
  assert.equal(tool.execute({decisions}).score, 56.54);
  assert.equal(button.disabled, false);
  // Test the optimizer controls with a short search, avoiding a second full run.
  ctx.CityOptimizer.searchPlans = function* () {
    yield { checked: 500, best: null };
    return { checked: 501, best: { decisions, score: 56.54307, cost: 95 } };
  };
  elements.get('#reset').handlers.click();
  const empty = vm.runInContext('JSON.stringify(state)', ctx);
  await elements.get('#optimize').handlers.click();
  assert.equal(vm.runInContext('JSON.stringify(state)', ctx), empty);
  assert.equal(elements.get('#optimize-apply').hidden, false);
  elements.get('#optimize-apply').handlers.click();
  assert.equal(vm.runInContext('compute(state).score.toFixed(2)', ctx), '56.54');
  const pending = elements.get('#optimize').handlers.click();
  elements.get('#optimize-cancel').handlers.click();
  await pending;
  assert.equal(elements.get('#optimize').disabled, false);
  assert.equal(elements.get('#optimize-apply').hidden, true);
  tool.execute({decisions});
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
  elements.get('#config-budget').value = '80';
  elements.get('#configuration-form').handlers.input({target:elements.get('#config-budget')});
  assert.equal(elements.get('#simulation').hidden, true);
  assert.equal(button.disabled, true);
  assert.equal(elements.get('#optimize-apply').hidden, true);
  assert.throws(()=>tool.execute({decisions}));
  elements.get('#configuration-form').handlers.submit({preventDefault(){}});
  assert.equal(elements.get('#simulation').hidden, false);
  assert.equal(elements.get('#summary-budget').textContent, '80 ед.');
  assert.throws(()=>tool.execute({decisions}));
  elements.get('#config-city').value='almaty';
  elements.get('#config-city').handlers.change();
  elements.get('#config-example').handlers.click();
  assert.equal(elements.get('#simulation').hidden,true);
  elements.get('#configuration-form').handlers.submit({preventDefault(){}});
  assert.equal(elements.get('#simulation').hidden,false);
  assert.equal(elements.get('#inputs-title').textContent,'Входные данные: Алматы');
  elements.get('#config-city').value='';
  elements.get('#config-city').handlers.change();
  elements.get('#config-example').handlers.click();
  assert.equal(elements.get('#simulation').hidden,true);
  assert.equal(elements.get('#config-budget').value,'');
  elements.get('#configuration-form').handlers.submit({preventDefault(){}});
  assert.match(elements.get('#config-errors').textContent,/бюджет/);
});
