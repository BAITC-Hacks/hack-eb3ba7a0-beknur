/* The browser sends decisions only; the server owns credentials and calculation. */
function setupAI(getScenario) {
  const button = document.querySelector('#ai-analyze');
  const status = document.querySelector('#ai-status');
  const result = document.querySelector('#ai-result');
  let controller;
  let version = 0;

  function reset() {
    version++;
    controller?.abort();
    controller = undefined;
    result.textContent = '';
    const error = getScenario().error;
    button.disabled = !!error || location.protocol === 'file:';
    status.textContent = location.protocol === 'file:'
      ? 'Для AI-анализа запустите npm start и откройте http://127.0.0.1:3000.'
      : error ? 'Выберите пять допустимых решений.' : 'Сценарий готов к AI-анализу.';
  }

  button.addEventListener('click', async () => {
    const { picks, error } = getScenario();
    if (error || button.disabled) return;
    const requestVersion = ++version;
    const requestController = new AbortController();
    controller = requestController;
    const timeout = setTimeout(() => requestController.abort(), 55000);
    button.disabled = true;
    result.textContent = '';
    status.textContent = 'ИИ анализирует сценарий…';
    try {
      const response = await fetch('/api/analyze', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ decisions: picks }),
        signal: requestController.signal,
      });
      if (!response.headers.get('content-type')?.includes('application/json')) {
        throw new Error('AI-сервер недоступен. Запустите проект через npm start.');
      }
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Не удалось получить AI-анализ.');
      if (typeof data.analysis !== 'string' || !data.analysis.trim()) throw new Error('ИИ вернул пустой ответ.');
      if (requestVersion !== version) return;
      result.textContent = data.analysis;
      status.textContent = 'AI-анализ готов. Числовая оценка рассчитана симулятором.';
    } catch (error) {
      if (requestVersion !== version) return;
      status.textContent = (error.name === 'AbortError'
        ? 'Время ожидания истекло. Попробуйте ещё раз.'
        : error instanceof TypeError ? 'Нет связи с сервером. Проверьте, что npm start запущен.' : error.message)
        + ' Расчёт и базовое объяснение доступны выше.';
    } finally {
      clearTimeout(timeout);
      if (requestVersion === version) {
        controller = undefined;
        button.disabled = !!getScenario().error;
      }
    }
  });
  return reset;
}
