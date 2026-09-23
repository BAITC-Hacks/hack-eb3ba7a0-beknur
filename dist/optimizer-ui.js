function setupOptimizer(applyPlan, getModel = () => CityModel) {
  const start = document.querySelector('#optimize');
  const cancel = document.querySelector('#optimize-cancel');
  const apply = document.querySelector('#optimize-apply');
  const status = document.querySelector('#optimize-status');
  const preview = document.querySelector('#optimize-result');
  let version = 0, best = null;
  function reset() {
    version++;
    best = null;
    start.disabled = !getModel();
    cancel.hidden = true;
    apply.hidden = true;
    preview.textContent = '';
    status.textContent = getModel() ? 'Можно подобрать план по вашим данным.' : 'Сначала заполните и примените настройки.';
  }

  cancel.addEventListener('click', () => {
    version++;
    start.disabled = false;
    cancel.hidden = true;
    status.textContent = 'Подбор отменён. Ваш план не изменён.';
  });
  start.addEventListener('click', async () => {
    if (start.disabled || !getModel()) return;
    const model = getModel();
    const request = ++version;
    best = null;
    apply.hidden = true;
    preview.textContent = '';
    start.disabled = true;
    cancel.hidden = false;
    status.textContent = 'Перебираем допустимые планы…';
    const search = CityOptimizer.searchPlans(model.measures, model);
    try {
      while (request === version) {
        await new Promise(resolve => setTimeout(resolve, 0));
        if (request !== version) break;
        const step = search.next();
        const { checked, best: candidate } = step.value;
        status.textContent = `Проверено планов: ${checked.toLocaleString('ru-RU')}. Лучший Score: ${candidate ? candidate.score.toFixed(2) : '—'}.`;
        if (!step.done) continue;
        if (!candidate) { status.textContent = 'Допустимый план не найден.'; break; }
        best = candidate;
        status.textContent = `Подбор завершён: проверено ${checked.toLocaleString('ru-RU')} планов. Максимальный Score ${best.score.toFixed(2)} (${(best.score - model.base.score)>=0?'+':''}${(best.score - model.base.score).toFixed(2)} к базе). Стоимость ${best.cost} из ${model.budget}.`;
        preview.textContent = best.decisions.map(p => `${p.id} · ${model.byId(p.id).name} — ${p.district || 'весь город'}`).join('\n');
        apply.hidden = false;
        break;
      }
    } catch {
      if (request === version) status.textContent = 'Не удалось завершить подбор. Попробуйте ещё раз.';
    } finally {
      search.return();
      if (request === version) { start.disabled = false; cancel.hidden = true; }
    }
  });
  apply.addEventListener('click', () => {
    if (!best || !getModel()) return;
    applyPlan(best.decisions);
    status.textContent = `План применён. Score ${best.score.toFixed(2)}. Теперь можно запросить AI-анализ или изменить решения вручную.`;
  });
  reset();
  return reset;
}
