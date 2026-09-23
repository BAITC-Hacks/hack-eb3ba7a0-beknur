const { test } = require('node:test');
const assert = require('node:assert/strict');
const { searchPlans } = require('../dist/optimizer.js');
const model = require('../dist/city-model.js');

function finish(search) {
  let step;
  do { step = search.next(); } while (!step.done);
  return step.value;
}

test('full catalog optimum is valid and agrees with the original calculation', () => {
  const { best, checked } = finish(searchPlans());
  assert.equal(checked, 694395);
  assert.equal(model.validate(best.decisions).error, '');
  assert.equal(best.cost, 98);
  assert.equal(best.score, model.compute(best.decisions).score);
  assert.ok(Math.abs(best.score - 57.236735) < 1e-10);
  assert.ok(best.score > 56.54307);
  assert.deepEqual(best.decisions.map(p => p.id), ['M2', 'M3', 'M8', 'M9', 'M14']);
});

test('pruned search matches independent enumeration including conflicts and synergies', () => {
  // Independent bit-mask enumeration, with no budget/category/conflict pruning.
  const catalog = model.measures.filter(m => ['M1','M2','M3','M4','M7','M9'].includes(m.id));
  let count = 0, maximum = -Infinity, cheapest = Infinity;
  for (let mask = 0; mask < 2 ** catalog.length; mask++) {
    const subset = catalog.filter((_, i) => mask & (1 << i));
    if (subset.length !== 5) continue;
    const local = subset.filter(m => m.type === 'Район');
    for (let assignment = 0; assignment < 5 ** local.length; assignment++) {
      let encoded = assignment;
      const picks = subset.map(m => {
        if (m.type === 'Город') return { id: m.id, district: '' };
        const district = model.districts[encoded % 5].name;
        encoded = Math.floor(encoded / 5);
        return { id: m.id, district };
      });
      const check = model.validate(picks);
      if (check.error) continue;
      count++;
      const score = model.compute(picks).score;
      if (score > maximum + 1e-10) { maximum = score; cheapest = check.cost; }
      else if (Math.abs(score - maximum) <= 1e-10) cheapest = Math.min(cheapest, check.cost);
    }
  }
  const actual = finish(searchPlans(catalog));
  assert.ok(count > 0);
  assert.equal(actual.checked, count);
  assert.ok(Math.abs(actual.best.score - maximum) < 1e-10);
  assert.equal(actual.best.cost, cheapest);
  assert.deepEqual(finish(searchPlans([])), { checked: 0, best: null });
});
