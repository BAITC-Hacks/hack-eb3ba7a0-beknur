const { test } = require('node:test');
const assert = require('node:assert/strict');
const { once } = require('node:events');
const { createModel, validateConfig, categories, exampleConfig, base } = require('../dist/city-model.js');
const { searchPlans } = require('../dist/optimizer.js');
const { createServer } = require('../server.cjs');

function custom() {
  return { budget: 150, demoRules: false,
    districts: [{ name: 'Новый район', population: 1200, v: Array(10).fill(50) }],
    measures: categories.map((category,i)=>({id:`M${i+1}`,name:`Проект ${i+1}`,category,type:'Район',cost:30,lag:0,effects:{[['T1','E1','S1','B1','C1'][i]]:8}})),
  };
}

test('custom budget, districts and effects control calculation and optimizer', () => {
  const config = custom();
  const model = createModel(config);
  const decisions = model.measures.map(m=>({id:m.id,district:'Новый район'}));
  assert.equal(model.validate(decisions).error, '');
  assert.equal(model.compute([]).score, 50);
  assert.ok(Math.abs(model.compute(decisions).score-53.92)<1e-9);
  let step, search = searchPlans(model.measures,model);
  do { step=search.next(); } while(!step.done);
  assert.equal(step.value.checked,1);
  assert.equal(step.value.best.cost,150);
  assert.ok(Math.abs(step.value.best.score-53.92)<1e-9);
  assert.equal(base.score.toFixed(4),'52.5577');
  assert.ok(createModel({...config,budget:149}).validate(decisions).error);
  config.districts[0].v[0]=0;
  assert.equal(model.districts[0].v[0],50);
});

test('incomplete and malformed configuration is rejected before calculation', () => {
  const c=custom();
  for(const input of [undefined,{}, {...c,budget:null},{...c,budget:-1},{...c,budget:Infinity},
    {...c,districts:[]}, {...c,measures:[]}, {...c,districts:[{...c.districts[0],name:42}]},
    {...c,districts:[{...c.districts[0],population:0}]},
    {...c,districts:[{...c.districts[0],v:Array(10).fill(null)}]},
    {...c,districts:[{...c.districts[0],v:Array(10).fill(101)}]},
    {...c,districts:[c.districts[0],c.districts[0]]},
    {...c,measures:[null,...c.measures.slice(1)]},
    {...c,measures:[{...c.measures[0],effects:{T1:'8'}},...c.measures.slice(1)]},
    {...c,measures:[{...c.measures[0],lag:9},...c.measures.slice(1)]},
    {...c,measures:[{...c.measures[0],category:'Other'},...c.measures.slice(1)]},
  ]) assert.ok(validateConfig(input).errors.length);
  assert.equal(validateConfig(exampleConfig).errors.length,0);
});

test('population weights are normalized from user counts',()=>{
  const c=custom();
  c.districts.push({name:'Второй',population:3600,v:Array(10).fill(70)});
  const model=createModel(c);
  assert.deepEqual(model.districts.map(d=>d.pop),[.25,.75]);
  assert.equal(model.base.avg,65);
});

test('API requires configuration and recomputes custom data without changing other requests', async t => {
  const seen=[];
  const server=createServer({apiKey:'test',cooldownMs:0,fetchImpl:async(_url,init)=>{
    seen.push(JSON.parse(JSON.parse(init.body).input));
    return Response.json({status:'completed',output:[{type:'message',content:[{type:'output_text',text:'Анализ'}]}]});
  }});
  server.listen(0,'127.0.0.1'); await once(server,'listening');
  t.after(()=>new Promise(resolve=>{server.close(resolve);server.closeAllConnections();}));
  const post=body=>fetch(`http://127.0.0.1:${server.address().port}/api/analyze`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  const config=custom(), decisions=config.measures.map(m=>({id:m.id,district:'Новый район'}));
  assert.equal((await post({decisions})).status,400);
  assert.equal((await post({decisions,config:{...config,budget:149}})).status,400);
  const reply=await post({decisions,config,score:999});
  assert.equal(reply.status,200);
  assert.equal((await reply.json()).score,53.92);
  assert.equal(seen[0].budget,150);
  assert.equal(seen[0].districts[0].name,'Новый район');
  assert.equal(seen[0].synergies.length,0);
  const exampleDecisions=[{id:'M7',district:'Нура'},{id:'M8',district:'Нура'},{id:'M10',district:'Нура'},{id:'M12'},{id:'M5',district:'Сарыарка'}];
  assert.equal((await post({config:exampleConfig,decisions:exampleDecisions})).status,200);
  assert.equal(seen[1].score,56.5431);
  assert.equal(seen[1].budget,100);
});
