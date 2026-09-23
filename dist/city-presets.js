(function(root){
  'use strict';
  const model=typeof module==='object'&&module.exports?require('./city-model.js'):root.CityModel;
  // Names are sourced below; numerical values are synthetic teaching data.
  // Astana intentionally retains the original five-district exercise.
  const cities=[
    {id:'astana',name:'Астана',note:'Исходный учебный шаблон Астаны сохранён без изменений.'},
    {id:'almaty',name:'Алматы',districts:['Алатауский','Алмалинский','Ауэзовский','Бостандыкский','Жетысуский','Медеуский','Наурызбайский','Турксибский'],note:'Все 8 административных районов Алматы. Ауэзовский — Әуезов, Алатауский — Алатау.',sources:['https://www.gov.kz/memleket/entities/almaty/activities/27965']},
    {id:'shymkent',name:'Шымкент',districts:['Абай','Әл-Фараби','Еңбекші','Қаратау','Тұран'],note:'5 административных районов Шымкента.',sources:['https://shymkent.kgd.gov.kz/ru/content/spisok-nalogoplatelshchikov-podlezhashchih-prinuditelnoy-likvidacii-soglasno-normam-stati--0','https://adilet.zan.kz/rus/docs/V22E0029189']},
    {id:'karaganda',name:'Караганда',districts:['Район имени Казыбек би','Район Әлихан Бөкейхан'],note:'2 административных района Караганды.',sources:['https://www.gov.kz/memleket/entities/karaganda-rayon-im-kazybek-bi?lang=kk','https://www.gov.kz/memleket/entities/karaganda-alikhan-bokeikhan']},
    {id:'aktobe',name:'Актобе',districts:['Астана','Алматы'],note:'2 административных района Актобе: Астана и Алматы.',sources:['https://adilet.zan.kz/rus/docs/G25CA00383M']},
    {id:'atyrau',name:'Атырау',districts:['Нурсая','Жилгородок','Жумыскер','Авангард'],note:'Выборка из 4 реальных микрорайонов Атырау; не полный перечень территорий и не административное деление.',sources:['https://law.gov.kz/api/documents/161791/rus/13.12.2021/download/pdf']},
    {id:'pavlodar',name:'Павлодар',districts:['Достык','Жастар','Усольский','Восточный'],note:'Выборка из 4 реальных микрорайонов Павлодара; не полный перечень территорий и не административное деление.',sources:['https://www.gov.kz/memleket/entities/pavlodar/press/news/details/759303?lang=ru']},
    {id:'oskemen',name:'Усть-Каменогорск',districts:['КШТ','Аблакетка','Согра','Защита','Куленова'],note:'Выборка из 5 реальных микрорайонов Усть-Каменогорска; не полный перечень территорий и не административное деление.',sources:['https://www.gov.kz/memleket/entities/emer/press/news/details/492661?lang=ru']},
    {id:'taraz',name:'Тараз',districts:['Әулиеата','Жібек жолы'],note:'2 административных района Тараза.',sources:['https://www.gov.kz/memleket/entities/zhambyl-taraz/press/news/details/1148031']},
    {id:'semey',name:'Семей',districts:['Карагайлы','Юность','Жоламан','Восточный','Степной'],note:'Выборка из 5 реальных жилых районов/микрорайонов Семея; не полный перечень территорий и не административное деление.',sources:['https://www.gov.kz/uploads/2024/3/19/f791cb6b3ed23b6afdaebf7d4ed875ce_original.25794169.pdf']},
  ];
  function load(id){
    const index=cities.findIndex(c=>c.id===id);
    if(index<0)throw new Error('Выберите город из списка.');
    const config=JSON.parse(JSON.stringify(model.exampleConfig));
    config.cityName=cities[index].name;
    if(id!=='astana'){
      config.districts=cities[index].districts.map((name,i)=>{
        const d=model.exampleConfig.districts[i%model.exampleConfig.districts.length];
        return {name,population:18000+i*3000,
          v:d.v.map((v,k)=>Math.max(20,Math.min(90,v+((index*3+i*2+k)%13)-6)))};
      });
    }
    return config;
  }
  const api={cities,load};
  if(typeof module==='object'&&module.exports)module.exports=api;else root.CityPresets=api;
})(globalThis);
