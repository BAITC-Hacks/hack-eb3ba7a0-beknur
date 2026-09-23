'use strict';
const http = require('node:http');
const { readFile } = require('node:fs/promises');
const path = require('node:path');
const { validate, compute, base, byId, keys } = require('./dist/city-model.js');

function loadConfig() {
  try { process.loadEnvFile(path.join(__dirname, '.env')); }
  catch (error) { if (error.code !== 'ENOENT') throw new Error('Не удалось прочитать .env.'); }
  return { apiKey: process.env.OPENAI_API_KEY?.trim(), model: process.env.OPENAI_MODEL?.trim() || 'gpt-6-luna' };
}

const round = n => Number(n.toFixed(4));
function factsFor(picks, cost) {
  const result = compute(picks);
  return {
    horizonQuarters: 8, budget: 100, cost, remaining: 100 - cost,
    score: round(result.score), baselineScore: round(base.score), delta: round(result.score - base.score),
    average: round(result.avg), minimum: round(result.min), criticalCount: result.critical,
    formula: '0.7 * populationWeightedAverage + 0.3 * minimumDistrictScore - count(indicators < 40)',
    indicators: Object.fromEntries(keys.map((k, i) => [k, ['Разгрузка дорог', 'Доступность общественного транспорта', 'Озеленение', 'Качество воздуха', 'Школы и детсады', 'Поликлиники', 'Безопасность улиц', 'Безопасность дорожного движения', 'Надёжность ЖКХ', 'Скорость решения обращений'][i]])),
    decisions: picks.map(p => {
      const m = byId(p.id);
      const alone = compute([p]);
      return { ...p, name: m.name, category: m.category, cost: m.cost, lag: m.lag,
        realizedEffects: Object.fromEntries(Object.entries(m.effects).map(([k, v]) => [k, v * (8 - m.lag) / 8])),
        standaloneScoreDelta: round(alone.score - base.score) };
    }),
    synergies: [['M1', 'M2', 'T1'], ['M10', 'M12', 'B1'], ['M5', 'M6', 'E2']]
      .filter(([a, b]) => picks.some(p => p.id === a) && picks.some(p => p.id === b))
      .map(([a, b, indicator]) => ({ measures: [a, b], district: picks.find(p => p.id === a).district, indicator, bonus: 2 })),
    districts: result.data.map((d, i) => ({ name: d.name, populationShare: d.pop,
      scoreBefore: round(base.data[i].score), scoreAfter: round(d.score), delta: round(d.score - base.data[i].score),
      indicators: Object.fromEntries(keys.map((k, j) => [k, { before: base.data[i].v[j], after: d.v[j], delta: round(d.v[j] - base.data[i].v[j]), critical: d.v[j] < 40 }])) })),
  };
}

function send(res, status, data) {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff' });
  res.end(JSON.stringify(data));
}

async function readJson(req) {
  const chunks = [];
  let size = 0;
  // Drain oversized requests without retaining their contents.
  for await (const chunk of req) {
    size += chunk.length;
    if (size <= 16384) chunks.push(chunk);
  }
  if (size > 16384) throw Object.assign(new Error('Запрос слишком большой.'), { status: 413 });
  try { return JSON.parse(Buffer.concat(chunks).toString('utf8')); }
  catch { throw Object.assign(new Error('Ожидается корректный JSON.'), { status: 400 }); }
}

function createServer({ apiKey, model = 'gpt-6-luna', fetchImpl = fetch, timeoutMs = 45000, cooldownMs = 3000 } = {}) {
  let busy = false;
  let nextRequest = 0;
  const files = new Map([
    ['/', ['index.html', 'text/html']], ['/index.html', ['index.html', 'text/html']],
    ['/city-model.js', ['city-model.js', 'text/javascript']], ['/ai-ui.js', ['ai-ui.js', 'text/javascript']],
  ]);
  return http.createServer({ requestTimeout: 15000, headersTimeout: 10000 }, async (req, res) => {
    try {
      // This server is intentionally local. Reject foreign Host/Origin values.
      if (!/^(127\.0\.0\.1|localhost)(:\d+)?$/.test(req.headers.host || '')) return send(res, 403, { error: 'Разрешён только локальный доступ.' });
      const url = new URL(req.url, `http://${req.headers.host}`);
      if (url.pathname !== '/api/analyze') {
        const file = files.get(url.pathname);
        if (!file) return send(res, 404, { error: 'Не найдено.' });
        if (!['GET', 'HEAD'].includes(req.method)) return send(res, 405, { error: 'Метод не поддерживается.' });
        const contents = await readFile(path.join(__dirname, 'dist', file[0]));
        res.writeHead(200, { 'Content-Type': `${file[1]}; charset=utf-8`, 'Cache-Control': 'no-cache', 'X-Content-Type-Options': 'nosniff' });
        return res.end(req.method === 'HEAD' ? undefined : contents);
      }
      if (req.method !== 'POST') return send(res, 405, { error: 'Используйте POST.' });
      if ((req.headers.origin && req.headers.origin !== `http://${req.headers.host}`) || req.headers['sec-fetch-site'] === 'cross-site') return send(res, 403, { error: 'Запрос с другого сайта запрещён.' });
      if (req.headers['content-type']?.split(';')[0].trim().toLowerCase() !== 'application/json') return send(res, 415, { error: 'Ожидается application/json.' });
      const input = await readJson(req);
      const check = validate(input?.decisions);
      if (check.error) return send(res, 400, { error: check.error });
      if (!apiKey) return send(res, 503, { error: 'Задайте OPENAI_API_KEY в .env и перезапустите сервер.' });
      if (busy || Date.now() < nextRequest) return send(res, 429, { error: 'Подождите несколько секунд и повторите запрос.' });
      busy = true;
      nextRequest = Date.now() + cooldownMs;
      try {
        const facts = factsFor(check.picks, check.cost);
        const upstream = await fetchImpl('https://api.openai.com/v1/responses', {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${apiKey}`, 'Content-Type': 'application/json' },
          signal: AbortSignal.timeout(timeoutMs),
          body: JSON.stringify({ model, store: false, max_output_tokens: 1800,
            instructions: 'Ты аналитик учебного симулятора «Аким на 5 часов». Отвечай по-русски обычным текстом до 250 слов: итог, сильные стороны, риски и компромиссы, рекомендации. Используй только переданные расчёты; не считай и не придумывай числа, прогнозы или эффекты альтернативных сценариев. Можно округлять готовые числа до двух знаков. Все показатели направлены одинаково: выше лучше. standaloneScoreDelta — эффект одной меры отдельно; такие дельты нельзя складывать из-за синергий, минимума и порога 40. Рекомендации качественные, требуют отдельного перерасчёта. Сохраняй ровно пять решений и бюджет 100; остаток не даёт бонуса. Данные синтетические, это не официальный прогноз.',
            input: JSON.stringify(facts),
          }),
        });
        if (!upstream.ok) {
          const messages = { 401: 'OpenAI отклонил ключ. Проверьте OPENAI_API_KEY и перезапустите сервер.', 403: 'OpenAI запретил доступ. Проверьте права проекта и доступность API.', 404: 'Модель недоступна. Проверьте OPENAI_MODEL в .env.', 429: 'Лимит OpenAI исчерпан или превышена частота запросов. Проверьте квоту и биллинг.' };
          // Never return raw provider errors, headers, or credentials.
          await upstream.body?.cancel();
          return send(res, upstream.status === 429 ? 429 : 502, { error: messages[upstream.status] || 'OpenAI не смог обработать запрос. Попробуйте позже.' });
        }
        const data = await upstream.json();
        const analysis = (data.output || []).filter(item => item.type === 'message')
          .flatMap(item => item.content || []).filter(item => item.type === 'output_text')
          .map(item => item.text).join('\n').trim();
        if (data.status !== 'completed' || !analysis) return send(res, 502, { error: 'ИИ не сформировал полный ответ. Повторите запрос.' });
        return send(res, 200, { analysis, score: facts.score, cost: check.cost });
      } catch (error) {
        return send(res, error.name === 'TimeoutError' || error.name === 'AbortError' ? 504 : 502, { error: 'Не удалось дождаться ответа OpenAI. Проверьте соединение и попробуйте ещё раз.' });
      } finally { busy = false; }
    } catch (error) {
      if (!res.headersSent) send(res, error.status || 500, { error: error.status ? error.message : 'Ошибка сервера.' });
    }
  });
}

if (require.main === module) {
  try {
    const config = loadConfig();
    const port = Number(process.env.PORT || 3000);
    if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('PORT должен быть числом от 1 до 65535.');
    const server = createServer(config);
    server.on('error', error => { console.error(error.code === 'EADDRINUSE' ? `Порт ${port} занят. Задайте другой PORT в .env.` : 'Не удалось запустить сервер.'); process.exitCode = 1; });
    server.listen(port, '127.0.0.1', () => {
      console.log(`Аким на 5 часов: http://127.0.0.1:${port}`);
      console.log(config.apiKey ? 'Ключ загружен. AI-анализ доступен по кнопке.' : 'Ключ не задан. Расчёт работает; AI-анализ требует OPENAI_API_KEY в .env.');
    });
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}

module.exports = { createServer, loadConfig, factsFor };
