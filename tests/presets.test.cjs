const {test}=require('node:test');
const assert=require('node:assert/strict');
const {cities,load}=require('../dist/city-presets.js');
const {createModel,validateConfig}=require('../dist/city-model.js');
const {factsFor}=require('../server.cjs');

test('all city presets are valid, editable, isolated teaching scenarios',()=>{
  for(const city of cities){
    const config=load(city.id);
    assert.equal(config.cityName,city.name);
    assert.deepEqual(validateConfig(config).errors,[]);
    const model=createModel(config);
    const picks=[{id:'M7',district:model.districts[model.districts.length-1].name},{id:'M8',district:model.districts[model.districts.length-1].name},{id:'M10',district:model.districts[model.districts.length-1].name},{id:'M12'},{id:'M5',district:model.districts[0].name}];
    assert.equal(model.validate(picks).error,'');
    assert.equal(factsFor(picks,95,model).cityName,city.name);
    config.budget=1;config.districts[0].v[0]=0;config.measures[0].cost=999;
    assert.equal(load(city.id).budget,100);
    assert.notEqual(load(city.id).districts[0].v[0],0);
    assert.equal(load(city.id).measures[0].cost,18);
  }
  assert.throws(()=>load('unknown'));
});

test('presets have city-specific districts and preserve the Astana exercise',()=>{
  const original=require('../dist/city-model.js').exampleConfig;
  assert.deepEqual(load('astana').districts,original.districts);
  assert.deepEqual(load('astana').measures,original.measures);
  assert.deepEqual(load('almaty').districts.map(d=>d.name),['Алатауский','Алмалинский','Ауэзовский','Бостандыкский','Жетысуский','Медеуский','Наурызбайский','Турксибский']);
  for(const id of ['karaganda','aktobe','taraz'])assert.equal(load(id).districts.length,2);
  for(const city of cities.filter(c=>c.id!=='astana')){
    assert.ok(city.sources.length);
    assert.ok(load(city.id).districts.every(d=>!d.name.includes('условный')));
  }
});
