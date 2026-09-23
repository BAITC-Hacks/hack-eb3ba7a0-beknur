
let activeModel=null;
let {keys,weights,districts,measures,byId,clip,compute,base,validate}=CityModel;
const esc=value=>String(value).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const state=Array.from({length:5},()=>({id:'',district:''}));
const fmt=n=>n.toFixed(2);

const directions=[
 {category:'Транспорт',label:'Транспорт',indices:[0,1],names:['Разгрузка дорог','Доступность общественного транспорта']},
 {category:'Экология',label:'Озеленение и экология',indices:[2,3],names:['Озеленение','Качество воздуха']},
 {category:'Соцсфера',label:'Социальная инфраструктура',indices:[4,5],names:['Школы и детсады','Поликлиники и первичная медпомощь']},
 {category:'Безопасность',label:'Безопасность',indices:[6,7],names:['Безопасность улиц','Безопасность дорожного движения']},
 {category:'Сервисы',label:'Городской сервис',indices:[8,9],names:['Надёжность ЖКХ','Скорость решения обращений']}
];
const indicatorNames=Object.fromEntries(directions.flatMap(g=>g.indices.map((k,i)=>[keys[k],g.names[i]])));
function renderInputs(){
 document.querySelector('#initial-indicators').innerHTML=directions.map(g=>`<article class="input-card"><h3>${g.label}</h3><table><thead><tr><th scope="col">Район</th>${g.indices.map((k,i)=>`<th scope="col">${keys[k]} · ${g.names[i]}</th>`).join('')}</tr></thead><tbody>${districts.map(d=>`<tr><th scope="row">${esc(d.name)} (${Math.round(d.pop*100)}%)</th>${g.indices.map(k=>`<td class="${d.v[k]<40?'critical':''}">${d.v[k]}${d.v[k]<40?' !':''}</td>`).join('')}</tr>`).join('')}</tbody></table></article>`).join('');
 document.querySelector('#catalog-body').innerHTML=measures.map(m=>`<tr><th scope="row">${m.id}</th><td>${directions.find(g=>g.category===m.category).label}</td><td>${esc(m.name)}</td><td>${m.cost} ед.</td><td>${m.type==='Город'?'Весь город':'Один район'}</td><td>${m.lag}</td><td>${Object.entries(m.effects).map(([k,v])=>`${k} · ${indicatorNames[k]}: ${v>0?'+':''}${v}`).join('<br>')}</td></tr>`).join('');
}
function validation(){return activeModel ? {...validate(state),config:activeModel.config} : {picks:[],cost:0,error:'Введите бюджет, районы и мероприятия и примените настройки.'}}
const resetAI=setupAI(()=>validation());
const resetOptimizer=setupOptimizer(decisions=>{const check=validate(decisions);if(check.error)throw Error(check.error);check.picks.forEach((p,i)=>Object.assign(state[i],p));build();update()},()=>activeModel);
function build(){document.querySelector('#decisions').innerHTML=state.map((p,i)=>{const m=byId(p.id);return `<div class="decision"><span class="num">${i+1}</span><div class="fields"><div><div class="label">Мероприятие</div><select aria-label="Мероприятие ${i+1}" data-i="${i}" data-field="id"><option value="">Выберите мероприятие</option>${measures.map(x=>`<option value="${x.id}" ${p.id===x.id?'selected':''}>${x.id} · ${esc(x.name)} · ${x.cost} ед.</option>`).join('')}</select></div><div><div class="label">Район</div><select aria-label="Район для решения ${i+1}" data-i="${i}" data-field="district" ${!m||m.type==='Город'?'disabled':''}><option value="">Выберите район</option>${districts.map(d=>`<option ${p.district===d.name?'selected':''}>${esc(d.name)}</option>`).join('')}</select></div></div><div class="detail">${m?`${m.category} · ${m.type==='Город'?'весь город':'один район'} · лаг ${m.lag} кв.<div class="cost">${m.cost} ед.</div>`:'Стоимость и лаг появятся после выбора'}</div></div>`}).join('');document.querySelectorAll('select[data-i]').forEach(el=>el.addEventListener('change',()=>{const i=Number(el.dataset.i),field=el.dataset.field;state[i][field]=el.value;if(field==='id')state[i].district='';build();update()}))}
function update(){resetAI();if(!activeModel)return;document.querySelector('#direction-counts').innerHTML=directions.map(g=>`<span>${g.label}: ${state.filter(p=>byId(p.id)?.category===g.category).length}</span>`).join('');const {picks,cost,error}=validation(),valid=!error;document.querySelector('#left').textContent=Number((activeModel.budget-cost).toFixed(2));document.querySelector('#left').style.color=cost>activeModel.budget?'#ffb78e':'';document.querySelector('#count').textContent=`Выбрано ${picks.length} из 5 · Стоимость ${cost}`;const err=document.querySelector('#error');err.style.display=error?'block':'none';err.textContent=error;const result=valid?compute(picks):base;document.querySelector('#score').innerHTML=valid?`${fmt(result.score)}<small> / 100</small>`:'<span class="scoreempty">Ожидает решения</span>';document.querySelector('#delta').textContent=valid?`Изменение ${result.score-base.score>=0?'+':''}${fmt(result.score-base.score)} к базе ${fmt(base.score)}`:`Базовый Score ${fmt(base.score)}`;document.querySelector('#meter').style.width=valid?clip(result.score)+'%':clip(base.score)+'%';const diffs=result.data.map((d,i)=>({name:d.name,delta:d.score-base.data[i].score,score:d.score}));const top=[...diffs].sort((a,b)=>b.delta-a.delta)[0],weak=[...diffs].sort((a,b)=>a.score-b.score)[0];document.querySelector('#analysis').textContent=valid?`За 8 кварталов средний балл по населению — ${fmt(result.avg)}, слабейший район — ${weak.name} (${fmt(result.min)}). Критических показателей ниже 40: ${result.critical}. Наибольший прирост — ${top.name} (${top.delta>=0?'+':''}${fmt(top.delta)}).`:'Числа рассчитаны по исходному синтетическому набору. Сценарий получит балл только после пяти допустимых решений.';let insights=[];if(valid){const areas=[['транспорта',[0,1]],['экологии',[2,3]],['соцсферы',[4,5]],['безопасности',[6,7]],['сервисов',[8,9]]].map(([name,inds])=>({name,delta:result.data.reduce((s,d,j)=>s+d.pop*inds.reduce((a,k)=>a+(d.v[k]-districts[j].v[k])*weights[k],0),0)})).sort((a,b)=>b.delta-a.delta);insights.push(`Сильная сторона: наибольший взвешенный вклад у ${areas[0].name} (+${fmt(areas[0].delta)}).`);insights.push(`Компромисс: самый слабый район — ${weak.name}; его результат влияет на 30% формулы.`);if(result.critical)insights.push(`Риск: ${result.critical} показателя(ей) остаются ниже 40. Сосредоточьтесь на этих провалах.`);else insights.push('Критических показателей ниже 40 не осталось.');const synergies=(activeModel.config.demoRules?[['M1','M2'],['M10','M12'],['M5','M6']]:[]).filter(([a,b])=>picks.some(p=>p.id===a)&&picks.some(p=>p.id===b));if(synergies.length)insights.push(`Синергии: ${synergies.map(x=>x.join(' + ')).join(', ')}.`);if(cost<activeModel.budget)insights.push(`Неизрасходованный остаток ${Number((activeModel.budget-cost).toFixed(2))} ед. не даёт бонуса.`)}else insights=['Проверьте правило пяти решений, бюджет, районы, повторы и несовместимости.'];document.querySelector('#insights').innerHTML=insights.map(t=>`<li>${esc(t)}</li>`).join('');const group=[[0,1],[2,3],[4,5],[6,7],[8,9]];document.querySelector('#districtbody').innerHTML=result.data.map((d,i)=>`<tr><td><strong>${esc(d.name)}</strong></td><td>${Math.round(d.pop*100)}%</td>${group.map(g=>{const before=g.reduce((s,k)=>s+districts[i].v[k],0)/2,after=g.reduce((s,k)=>s+d.v[k],0)/2;return `<td>${fmt(before)} <span class="${after>before?'up':after<before?'down':''}">→ ${fmt(after)}</span></td>`}).join('')}<td><strong>${fmt(d.score)}</strong></td></tr>`).join('')};document.querySelector('#reset').addEventListener('click',()=>{state.forEach(p=>{p.id='';p.district=''});build();update()});
if(document.modelContext?.registerTool){try{Promise.resolve(document.modelContext.registerTool({name:'set_city_scenario',title:'Установить сценарий',description:'Задать пять мероприятий и районы, проверить правила и показать результат в симуляторе.',inputSchema:{type:'object',properties:{decisions:{type:'array',minItems:5,maxItems:5,items:{type:'object',properties:{id:{type:'string'},district:{type:'string'}},required:['id'],additionalProperties:false}}},required:['decisions'],additionalProperties:false},annotations:{readOnlyHint:false},execute(input){if(!activeModel)throw Error('Сначала заполните и примените настройки.');if(!Array.isArray(input?.decisions)||input.decisions.length!==5)throw Error('Нужно ровно пять решений');const check=validate(input.decisions);if(check.error)throw Error(check.error);check.picks.forEach((x,i)=>Object.assign(state[i],x));build();update();return {score:Number(compute(check.picks).score.toFixed(2)),cost:check.cost,remaining:activeModel.budget-check.cost}}})).catch(()=>{})}catch(e){}}

setupConfiguration({
 onChange(){
  activeModel=null;
  state.forEach(p=>{p.id='';p.district=''});
  document.querySelector('#simulation').hidden=true;
  resetAI();resetOptimizer();
 },
 onApply(model){
  activeModel=model;
  document.querySelector('#inputs-title').textContent=`Входные данные: ${model.config.cityName || 'свой город'}`;
  ({keys,weights,districts,measures,byId,clip,compute,base,validate}=model);
  state.forEach(p=>{p.id='';p.district=''});
  document.querySelector('#summary-budget').textContent=`${model.budget} ед.`;
  document.querySelector('#summary-districts').textContent=`${districts.length} районов`;
  document.querySelector('#summary-measures').textContent=`${measures.length} мероприятий`;
  document.querySelector('#budget-total').textContent=`Осталось из ${model.budget}`;
  document.querySelector('#plan-rules').textContent=`Ровно 5 разных мер, не более 2 из одного направления; бюджет — до ${model.budget} ед.`;
  document.querySelector('#catalog-count').textContent=`Доступно ${measures.length} мер`;
  document.querySelector('#special-rules').textContent=model.config.demoRules?'Правила примера включены: M1 + M3 запрещены вместе; M4 + M7 и M5 + M13 — в одном районе. Синергии M1 + M2: T1 +2; M10 + M12: B1 +2; M5 + M6: E2 +2 в районе первой меры.':'Для вашего каталога дополнительные несовместимости и синергии не заданы.';
  renderInputs();build();update();resetOptimizer();
  document.querySelector('#simulation').hidden=false;
 }
});
