(function(root){
'use strict';
const keys=['T1','T2','E1','E2','S1','S2','B1','B2','C1','C2'], weights=[.10,.10,.09,.11,.11,.11,.09,.09,.10,.10];
const districts=[{name:'Есиль',pop:.27,v:[45,62,68,72,48,55,78,60,75,70]},{name:'Алматы',pop:.24,v:[40,75,50,55,60,65,62,52,50,60]},{name:'Сарыарка',pop:.20,v:[50,70,42,40,62,68,58,55,45,55]},{name:'Байконур',pop:.13,v:[52,68,55,50,58,60,52,58,55,58]},{name:'Нура',pop:.16,v:[55,40,45,65,38,35,55,50,60,50]}];
const measures=[['M1','Транспорт','Выделенные полосы для автобусов','Район',18,2,{T1:6,T2:9}],['M2','Транспорт','Умные светофоры','Город',22,2,{T1:4,B2:3}],['M3','Транспорт','Линия ЛРТ / расширение','Район',30,4,{T1:16,T2:20,E2:4}],['M4','Экология','Парк / сквер','Район',15,2,{E1:12,E2:3,B1:2}],['M5','Экология','Перевод частного сектора на чистое топливо','Район',25,3,{E2:14,C1:4}],['M6','Экология','Городская программа озеленения','Город',20,4,{E1:5,E2:3}],['M7','Соцсфера','Школа + детсад','Район',24,3,{S1:16}],['M8','Соцсфера','Центр семейного здоровья','Район',20,3,{S2:14}],['M9','Соцсфера','Дворовые спорт-хабы','Район',10,1,{S1:3,S2:3,B1:3}],['M10','Безопасность','Освещение и камеры Safe City','Район',12,1,{B1:12,B2:2}],['M11','Безопасность','Безопасные переходы и школьные зоны','Район',10,1,{B2:12,T1:-2}],['M12','Сервисы','Единая цифровая платформа обращений','Город',14,1,{C2:5}],['M13','Сервисы','Модернизация тепло- и водосетей','Район',28,4,{C1:18,E2:2}],['M14','Сервисы','Аварийные бригады ЖКХ и оповещение','Город',16,1,{C1:5,C2:2}]].map(([id,category,name,type,cost,lag,effects])=>({id,category,name,type,cost,lag,effects}));

const categories=['Транспорт','Экология','Соцсфера','Безопасность','Сервисы'];
const exampleConfig={budget:100,demoRules:true,districts:districts.map(d=>({name:d.name,population:Math.round(d.pop*100000),v:[...d.v]})),measures};
function validateConfig(input){
 const errors=[];
 const text=x=>typeof x==='string'&&x.trim().length>0&&x.trim().length<=100;
 const number=(x,min,max)=>typeof x==='number'&&Number.isFinite(x)&&x>=min&&x<=max;
 if(!input||typeof input!=='object')return {errors:['Введите бюджет, районы и мероприятия.']};
 if(input.cityName!==undefined&&!text(input.cityName))errors.push('Название города должно содержать от 1 до 100 символов.');
 if(!number(input.budget,0.01,1000000000))errors.push('Введите бюджет от 0,01 до 1 000 000 000.');
 if(input.demoRules!==undefined&&typeof input.demoRules!=='boolean')errors.push('Некорректный режим правил примера.');
 const ds=Array.isArray(input.districts)?input.districts:[];
 const ms=Array.isArray(input.measures)?input.measures:[];
 if(ds.length<1||ds.length>10)errors.push('Добавьте от 1 до 10 районов.');
 if(ms.length<5||ms.length>20)errors.push('Добавьте от 5 до 20 мероприятий, чтобы выбрать пять решений.');
 if(ds.length>10||ms.length>20)return {errors};
 ds.forEach((d,i)=>{
  if(!d||!text(d.name))errors.push(`Район ${i+1}: введите название (до 100 символов).`);
  if(!d||!number(d.population,1,1000000000)||!Number.isInteger(d.population))errors.push(`Район ${i+1}: введите численность населения целым положительным числом.`);
  if(!d||!Array.isArray(d.v)||d.v.length!==10||d.v.some(v=>!number(v,0,100)))errors.push(`Район ${i+1}: заполните все 10 показателей числами от 0 до 100.`);
 });
 if(new Set(ds.map(d=>typeof d?.name==='string'?d.name.trim().toLowerCase():'')).size!==ds.length)errors.push('Названия районов не должны повторяться.');
 ms.forEach((m,i)=>{
  if(!m||typeof m.id!=='string'||!/^M[1-9][0-9]{0,3}$/.test(m.id))errors.push(`Мероприятие ${i+1}: некорректный ID.`);
  if(!m||!text(m.name))errors.push(`Мероприятие ${i+1}: введите название (до 100 символов).`);
  if(!m||!categories.includes(m.category))errors.push(`Мероприятие ${i+1}: выберите направление.`);
  if(!m||!['Район','Город'].includes(m.type))errors.push(`Мероприятие ${i+1}: выберите область действия.`);
  if(!m||!number(m.cost,0.01,1000000000))errors.push(`Мероприятие ${i+1}: введите положительную стоимость.`);
  if(!m||!Number.isInteger(m.lag)||!number(m.lag,0,8))errors.push(`Мероприятие ${i+1}: лаг должен быть целым числом от 0 до 8.`);
  if(!m?.effects||typeof m.effects!=='object'||Array.isArray(m.effects)||Object.entries(m.effects).some(([k,v])=>!keys.includes(k)||!number(v,-100,100)))errors.push(`Мероприятие ${i+1}: эффекты должны быть числами от −100 до 100.`);
 });
 if(new Set(ms.map(m=>m?.id)).size!==ms.length)errors.push('ID мероприятий не должны повторяться.');
 if(input.demoRules){
  for(const id of ['M1','M4','M5','M7','M10','M13']){
   const m=ms.find(m=>m?.id===id);
   if(m&&m.type!=='Район')errors.push(`Правила примера требуют районный тип для ${id}. Отключите правила примера для другого каталога.`);
  }
 }
 if(errors.length)return {errors};
 const config={budget:input.budget,demoRules:!!input.demoRules,
  ...(input.cityName!==undefined?{cityName:input.cityName.trim()}:{}),
  districts:ds.map(d=>({name:d.name.trim(),population:d.population,v:[...d.v]})),
  measures:ms.map(m=>({id:m.id,name:m.name.trim(),category:m.category,type:m.type,cost:m.cost,lag:m.lag,effects:{...m.effects}}))};
 return {errors:[],config};
}
function createModel(input){
 const checked=validateConfig(input);
 if(checked.errors.length)throw new Error(checked.errors.join(' '));
 const config=checked.config,budget=config.budget;
 const total=config.districts.reduce((s,d)=>s+d.population,0);
 const districts=config.districts.map(d=>({...d,pop:d.population/total}));
 const measures=config.measures;
const byId=id=>measures.find(m=>m.id===id), clip=n=>Math.max(0,Math.min(100,n));function compute(picks){let data=districts.map(d=>({...d,v:[...d.v]}));for(const p of picks){const m=byId(p.id),targets=m.type==='Город'?data:[data.find(d=>d.name===p.district)];for(const d of targets)for(const [key,effect] of Object.entries(m.effects))d.v[keys.indexOf(key)]+=effect*(8-m.lag)/8}const has=id=>picks.some(p=>p.id===id);for(const [a,b,key,bonus] of (config.demoRules ? [['M1','M2','T1',2],['M10','M12','B1',2],['M5','M6','E2',2]] : []))if(has(a)&&has(b)){const p=picks.find(p=>p.id===a),d=data.find(d=>d.name===p.district);d.v[keys.indexOf(key)]+=bonus}for(const d of data)d.v=d.v.map(clip);let critical=0;for(const d of data){d.score=d.v.reduce((s,v,i)=>s+v*weights[i],0);critical+=d.v.filter(v=>v<40).length}const avg=data.reduce((s,d)=>s+d.pop*d.score,0),min=Math.min(...data.map(d=>d.score));return {data,critical,avg,min,score:.7*avg+.3*min-critical}}const base=compute([]);
function validate(input){
if(!Array.isArray(input)||input.length!==5)return {picks:[],cost:0,error:'Нужно ровно пять решений.'};
if(input.some(p=>!p||typeof p!=='object'||Array.isArray(p)||typeof p.id!=='string'||(p.district!==undefined&&typeof p.district!=='string')))return {picks:[],cost:0,error:'Некорректный формат решения.'};
const picks=input.map(p=>({id:p.id,district:p.district??''})).filter(p=>p.id);
if(picks.some(p=>!byId(p.id)))return {picks:[],cost:0,error:'Неизвестное мероприятие.'};
if(picks.some(p=>p.district&&!districts.some(d=>d.name===p.district)))return {picks:[],cost:0,error:'Неизвестный район.'};
const cost=picks.reduce((s,p)=>s+byId(p.id).cost,0),counts={};for(const p of picks){const m=byId(p.id);counts[m.category]=(counts[m.category]||0)+1}let error='';if(cost>budget+1e-9)error=`Бюджет превышен на ${Number((cost-budget).toFixed(2))} ед.`;else if(new Set(picks.map(p=>p.id)).size!==picks.length)error='Каждое мероприятие можно выбрать только один раз.';else if(Object.entries(counts).some(([,n])=>n>2))error='Допускается не более двух мероприятий из одного направления.';else if(picks.some(p=>byId(p.id).type==='Район'&&!p.district))error='Для районного мероприятия укажите район.';else if(picks.some(p=>byId(p.id).type==='Город'&&p.district))error='Для общегородского мероприятия район не выбирается.';else if(config.demoRules&&picks.some(p=>p.id==='M1')&&picks.some(p=>p.id==='M3'))error='Выделенные полосы и ЛРТ несовместимы.';else for(const [a,b] of (config.demoRules ? [['M4','M7'],['M5','M13']] : [])){const pa=picks.find(p=>p.id===a),pb=picks.find(p=>p.id===b);if(pa&&pb&&pa.district===pb.district){error=`${a} и ${b} нельзя проводить в одном районе.`;break}}if(!error&&picks.length!==5)error=`Выберите ровно 5 мероприятий: сейчас ${picks.length}.`;return {picks,cost,error}};


return {keys,weights,categories,config,budget,districts,measures,byId,clip,compute,base,validate};
}
const api={...createModel(exampleConfig),createModel,validateConfig,exampleConfig};
if(typeof module==='object'&&module.exports)module.exports=api;else root.CityModel=api;
})(globalThis);
