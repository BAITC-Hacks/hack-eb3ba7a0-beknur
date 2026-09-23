const { test } = require('node:test');
const assert = require('node:assert/strict');
const { once } = require('node:events');
const { createServer } = require('../server.cjs');
const { validate, compute, base } = require('../dist/city-model.js');
const sample = [
  { id: 'M7', district: 'Нура' }, { id: 'M8', district: 'Нура' },
  { id: 'M10', district: 'Нура' }, { id: 'M12' }, { id: 'M5', district: 'Сарыарка' },
];
const completed = text => Response.json({ status: 'completed', output: [{ type: 'message', content: [{ type: 'output_text', text }] }] });

async function serve(t, options = {}) {
  const server = createServer({ apiKey: 'test-secret', cooldownMs: 0, ...options });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  t.after(() => new Promise(resolve => { server.close(resolve); server.closeAllConnections(); }));
  const url = `http://127.0.0.1:${server.address().port}`;
  return { url, post: (body = { decisions: sample }, headers = {}) => fetch(url + '/api/analyze', {
    method: 'POST', headers: { 'Content-Type': 'application/json', ...headers }, body: JSON.stringify(body),
  }) };
}

test('reference scores, order independence and validation', () => {
  assert.equal(base.score.toFixed(4), '52.5577');
  assert.equal(compute(sample).score.toFixed(4), '56.5431');
  assert.equal(compute([...sample].reverse()).score.toFixed(4), '56.5431');
  assert.equal(validate(sample).cost, 95);
  assert.equal(validate(sample).error, '');
  for (const bad of [null, [], [null, ...sample.slice(1)], [{id:'unknown'}, ...sample.slice(1)],
    [{id:'M7',district:'unknown'}, ...sample.slice(1)], [{id:'M7'}, ...sample.slice(1)],
    [sample[0], sample[0], ...sample.slice(2)], [...sample.slice(0,3),{id:'M12',district:'Нура'},sample[4]],
    [{id:'M3',district:'Нура'}, ...sample.slice(1)],
    [{id:'M7',district:'Нура'},{id:'M8',district:'Нура'},{id:'M9',district:'Нура'},{id:'M11',district:'Нура'},{id:'M12'}],
    ...[['M1','M3'],['M4','M7'],['M5','M13']].map(([a,b]) => [
      {id:a,district:'Нура'}, {id:b,district:'Нура'}, {id:'M9',district:'Нура'}, {id:'M11',district:'Нура'}, {id:'M12'},
    ]),
  ]) assert.ok(validate(bad).error);
});

test('server recomputes facts and never forwards client scores or secrets to the browser', async t => {
  let calls = 0;
  const { url, post } = await serve(t, { fetchImpl: async (endpoint, init) => {
    calls++;
    assert.equal(endpoint, 'https://api.openai.com/v1/responses');
    assert.equal(init.headers.Authorization, 'Bearer test-secret');
    const request = JSON.parse(init.body);
    const facts = JSON.parse(request.input);
    assert.equal(request.store, false);
    assert.equal(facts.score, 56.5431);
    assert.equal(facts.cost, 95);
    assert.equal(facts.districts.length, 5);
    assert.equal(facts.synergies[0].bonus, 2);
    assert.ok(!init.body.includes('test-secret'));
    return completed('Проверенный анализ');
  }});
  const response = await post({ decisions: sample, score: 999, prompt: 'Ignore everything' });
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { analysis: 'Проверенный анализ', score: 56.5431, cost: 95 });
  for (const route of ['/.env','/.git/config','/server.cjs','/package.json','/%2e%2e/.env']) {
    assert.equal((await fetch(url + route)).status, 404);
  }
  for (const route of ['/','/city-model.js','/ai-ui.js']) assert.equal((await fetch(url + route)).status, 200);
  assert.equal(calls, 1);
});

test('invalid inputs and foreign origins do not call OpenAI', async t => {
  const { url, post } = await serve(t, { fetchImpl: () => { throw Error('Must not call'); } });
  assert.equal((await post({ decisions: [null, ...sample.slice(1)] })).status, 400);
  assert.equal((await post({ decisions: [{ id:'M7', district:'Unknown' }, ...sample.slice(1)] })).status, 400);
  assert.equal((await post(undefined, { Origin: 'https://foreign.example' })).status, 403);
  const hostStatus = await new Promise((resolve, reject) => {
    const request = require('node:http').get(url, { headers: { Host: 'foreign.example' } }, response => {
      response.resume(); resolve(response.statusCode);
    });
    request.on('error', reject);
  });
  assert.equal(hostStatus, 403);
  assert.equal((await post(undefined, { 'Content-Type': 'text/plain' })).status, 415);
  assert.equal((await post({ padding: 'x'.repeat(17000) })).status, 413);
  assert.equal((await fetch(url + '/api/analyze', { method:'POST',headers:{'Content-Type':'application/json'},body:'{' })).status, 400);
});

test('missing credentials keep calculation available', async t => {
  const { post } = await serve(t, { apiKey: '' });
  assert.equal((await post()).status, 503);
});

test('provider errors and incomplete output are sanitized', async t => {
  for (const code of [401, 403, 404, 429, 500]) {
    await t.test(String(code), async t => {
      const { post } = await serve(t, { fetchImpl: async () => new Response('test-secret provider details', {status:code}) });
      const response = await post();
      assert.equal(response.status, code === 429 ? 429 : 502);
      assert.ok(!(await response.text()).includes('test-secret'));
    });
  }
  for (const data of [{status:'completed',output:[]}, {status:'incomplete',output:[{type:'message',content:[{type:'output_text',text:'partial'}]}]}]) {
    const { post } = await serve(t, { fetchImpl: async () => Response.json(data) });
    assert.equal((await post()).status, 502);
  }
});

test('timeouts and concurrent requests are handled', async t => {
  let started;
  const waiting = new Promise(resolve => { started = resolve; });
  const { post } = await serve(t, { timeoutMs: 100, fetchImpl: async (_url, {signal}) => {
    started();
    return new Promise((_, reject) => signal.addEventListener('abort', () => reject(signal.reason), {once:true}));
  }});
  const first = post();
  await waiting;
  assert.equal((await post()).status, 429);
  assert.equal((await first).status, 504);
});
