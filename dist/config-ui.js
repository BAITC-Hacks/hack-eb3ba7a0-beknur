function setupConfiguration({ onChange, onApply }) {
  const form = document.querySelector('#configuration-form');
  const budget = document.querySelector('#config-budget');
  const districtRows = document.querySelector('#config-districts');
  const measureRows = document.querySelector('#config-measures');
  const rules = document.querySelector('#config-demo-rules');
  const notice = document.querySelector('#config-notice');
  const message = document.querySelector('#config-errors');
  const city = document.querySelector('#config-city');
  const loadCity = document.querySelector('#config-example');
  const cityHint = document.querySelector('#config-city-hint');
  const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const names = ['Разгрузка дорог','Общественный транспорт','Озеленение','Качество воздуха','Школы и детсады','Поликлиники','Безопасность улиц','Безопасность дорог','Надёжность ЖКХ','Решение обращений'];
  let draft = { budget: null, demoRules: false, districts: [], measures: [] };
  city.innerHTML = '<option value="">Ввести вручную — свой город</option>' + CityPresets.cities.map(c=>`<option value="${c.id}">${esc(c.name)}</option>`).join('');
  city.value = '';
  function updateCityChoice() {
    loadCity.textContent = city.value ? 'Загрузить выбранный город' : 'Начать новый ручной набор';
    const selected = CityPresets.cities.find(c=>c.id===city.value);
    document.querySelector('#config-city-source').innerHTML = (selected?.sources||[]).map((url,i)=>`<a class="accent" href="${url}" target="_blank" rel="noopener noreferrer">Источник названий ${i+1}</a>`).join(' · ');
    cityHint.textContent = city.value
      ? `${selected.note} Бюджет, население, показатели и эффекты — учебные, не официальная статистика. Нажмите «Загрузить выбранный город», чтобы заполнить форму.`
      : 'Заполните поля ниже самостоятельно. Кнопка «Начать новый ручной набор» очистит текущую форму.';
  }
  city.addEventListener('change', updateCityChoice);
  updateCityChoice();
  const labels = ['Транспорт','Озеленение и экология','Социальная инфраструктура','Безопасность','Городской сервис'];
  const field = (group, index, key, label, value, type='text', extra='') => `<label>${label}<input data-group="${group}" data-index="${index}" data-key="${key}" type="${type}" value="${esc(value)}" ${extra}></label>`;
  const select = (index, key, label, value, options) => `<label>${label}<select data-group="measures" data-index="${index}" data-key="${key}"><option value="">Выберите…</option>${options.map(([v,l])=>`<option value="${esc(v)}" ${v===value?'selected':''}>${esc(l)}</option>`).join('')}</select></label>`;
  function changed() {
    onChange();
    notice.textContent = 'Введите бюджет, добавьте районы и не менее пяти мероприятий. Затем нажмите «Применить настройки». До этого симуляция недоступна.';
    message.textContent = '';
  }
  function render() {
    budget.value = draft.budget ?? '';
    rules.checked = draft.demoRules;
    districtRows.innerHTML = draft.districts.length ? draft.districts.map((d,i)=>`<fieldset class="config-card"><legend>Район ${i+1}</legend><div class="config-fields">${field('districts',i,'name','Название района',d.name,'text','maxlength="100"')}${field('districts',i,'population','Население, человек',d.population,'number','min="1" max="1000000000" step="1"')}</div><div class="config-indicators">${CityModel.keys.map((k,j)=>field('districts',i,k,`${k} · ${names[j]}`,d.v[j],'number','min="0" max="100" step="any"')).join('')}</div><button type="button" class="reset" data-remove="districts" data-index="${i}">Удалить район ${i+1}</button></fieldset>`).join('') : '<p class="input-note">Районы ещё не добавлены. Добавьте хотя бы один район и заполните его показатели.</p>';
    measureRows.innerHTML = draft.measures.length ? draft.measures.map((m,i)=>`<fieldset class="config-card"><legend>Мероприятие ${esc(m.id)}</legend><div class="config-fields">${field('measures',i,'name','Название',m.name,'text','maxlength="100"')}${select(i,'category','Направление',m.category,CityModel.categories.map((c,j)=>[c,labels[j]]))}${field('measures',i,'cost','Стоимость, ед.',m.cost,'number','min="0.01" max="1000000000" step="any"')}${select(i,'type','Область действия',m.type,[['Район','Один выбранный район'],['Город','Все районы']])}${field('measures',i,'lag','Лаг, кварталов (0–8)',m.lag,'number','min="0" max="8" step="1"')}</div><details><summary>Эффекты по показателям (0 — нет изменения)</summary><div class="config-indicators">${CityModel.keys.map((k,j)=>field('measures',i,k,`${k} · ${names[j]}`,m.effects[k]??0,'number','min="-100" max="100" step="any"')).join('')}</div></details><button type="button" class="reset" data-remove="measures" data-index="${i}">Удалить ${esc(m.id)}</button></fieldset>`).join('') : '<p class="input-note">Каталог пуст. Добавьте минимум пять мероприятий. Можно выбрать не более двух мер одного направления.</p>';
  }
  form.addEventListener('input', event => {
    const el = event.target;
    if (el === budget) draft.budget = el.value.trim()==='' ? null : Number(el.value);
    else if (el === rules) draft.demoRules = el.checked;
    else if (el.dataset.group) {
      const { group, index, key } = el.dataset;
      const row = draft[group]?.[Number(index)];
      if (!row) return;
      const value = el.type === 'number' ? (el.value.trim()==='' ? null : Number(el.value)) : el.value;
      if (CityModel.keys.includes(key)) {
        if (group === 'districts') row.v[CityModel.keys.indexOf(key)] = value;
        else row.effects[key] = value;
      } else row[key] = value;
    } else return;
    changed();
  });
  form.addEventListener('click', event => {
    const button = event.target.closest('button[data-remove]');
    if (!button) return;
    draft[button.dataset.remove].splice(Number(button.dataset.index),1);
    changed(); render();
  });
  document.querySelector('#config-add-district').addEventListener('click', () => {
    if (draft.districts.length >= 10) { message.textContent = 'Можно добавить не более 10 районов.'; return; }
    draft.districts.push({name:'',population:null,v:Array(10).fill(null)});
    changed(); render();
  });
  document.querySelector('#config-add-measure').addEventListener('click', () => {
    if (draft.measures.length >= 20) { message.textContent = 'Можно добавить не более 20 мероприятий.'; return; }
    const next = Math.max(0,...draft.measures.map(m=>Number(m.id.slice(1))))+1;
    draft.measures.push({id:`M${next}`,name:'',category:'',type:'Район',cost:null,lag:0,effects:{}});
    changed(); render();
  });
  loadCity.addEventListener('click', () => {
    draft = city.value ? CityPresets.load(city.value) : {budget:null,demoRules:false,districts:[],measures:[]};
    changed(); render();
    notice.textContent = city.value ? `${draft.cityName}: учебный набор загружен. Проверьте данные и нажмите «Применить настройки». Это не официальная статистика города.` : 'Ручной ввод: введите бюджет, добавьте районы и мероприятия.';
  });
  document.querySelector('#config-clear').addEventListener('click', () => {
    draft = {budget:null,demoRules:false,districts:[],measures:[]};
    city.value = ''; updateCityChoice();
    changed(); render();
  });
  form.addEventListener('submit', event => {
    event.preventDefault();
    const check = CityModel.validateConfig(draft);
    if (check.errors.length) {
      changed();
      message.textContent = check.errors.join('\n');
      return;
    }
    onApply(CityModel.createModel(check.config));
    notice.textContent = `${check.config.cityName || 'Свой город'}: настройки применены. Бюджет ${check.config.budget}, районов ${check.config.districts.length}, мероприятий ${check.config.measures.length}. Можно принимать решения ниже.`;
    message.textContent = '';
  });
  render();
  changed();
}
