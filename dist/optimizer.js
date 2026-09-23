(function (root) {
  'use strict';
  const defaultModel = typeof module === 'object' && module.exports ? require('./city-model.js') : root.CityModel;

  // Increasing catalog indices enumerate each set once. Every district assignment
  // is checked by the same validator and scored by the original calculation.
  function* searchPlans(catalog = defaultModel.measures, model = defaultModel) {
    let checked = 0, visited = 0, best = null;
    const selected = [], picks = [], counts = {};

    function* assign(index, cost) {
      if (index === selected.length) {
        visited++;
        if (visited % 500 === 0) yield { checked, best };
        const check = model.validate(picks);
        if (check.error) return;
        const score = model.compute(check.picks).score;
        checked++;
        if (!best || score > best.score + 1e-10 ||
            (Math.abs(score - best.score) <= 1e-10 && cost < best.cost)) {
          best = { decisions: check.picks, score, cost };
        }
        // Yield regularly to keep the interface responsive and permit cancellation.
        return;
      }
      const measure = selected[index];
      const targets = measure.type === 'Город' ? [''] : model.districts.map(d => d.name);
      for (const district of targets) {
        picks.push({ id: measure.id, district });
        yield* assign(index + 1, cost);
        picks.pop();
      }
    }

    function* choose(start, cost) {
      if (selected.length === 5) { yield* assign(0, cost); return; }
      const needed = 5 - selected.length;
      for (let i = start; i <= catalog.length - needed; i++) {
        const measure = catalog[i];
        if (cost + measure.cost > model.budget + 1e-9 || (counts[measure.category] || 0) >= 2) continue;
        if (model.config.demoRules && selected.some(m => (m.id === 'M1' && measure.id === 'M3') || (m.id === 'M3' && measure.id === 'M1'))) continue;
        selected.push(measure);
        counts[measure.category] = (counts[measure.category] || 0) + 1;
        yield* choose(i + 1, cost + measure.cost);
        counts[measure.category]--;
        selected.pop();
      }
    }
    yield* choose(0, 0);
    return { checked, best };
  }

  const api = { searchPlans };
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.CityOptimizer = api;
})(globalThis);
