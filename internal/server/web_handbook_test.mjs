import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {renderHandbook,newHandbookState,hostReference,effectText,itemName} from './web/handbook.js';
import {setLocale,t} from './web/i18n.js';

const json=async path=>JSON.parse(await readFile(new URL(path,import.meta.url),'utf8'));
const game=await json('../game/default.json'),catalog=await json('../game/items.json');
const cardConfig=await json('../cards/default.json'),specialties=await json('../game/specialties.json');
const hintConfig=await json('../hints/default.json');
const rules=hostReference({game,catalog:catalog.items.map(i=>({...i,price:i.fixed_price??1234})),cards:cardConfig,specialties,categories:catalog.categories,public_hint_definitions:hintConfig.plugins});

test('handbook renders every configured card, specialty and clue in both languages',()=>{
  try{for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    for(const section of ['items','cards','hints','specialties']){
      const definitions=section==='items'?rules.catalog.slice(0,6):section==='cards'?rules.cards:section==='hints'?rules.public_hint_definitions:rules.specialties;
      for(const def of definitions){
        const html=renderHandbook(rules,{...newHandbookState(section),entry:def.id,detail:true});
        assert.ok(html.includes('book-entry:'+def.id),def.id);
        assert.ok(html.includes('has-detail'));
        if(locale==='en')assert.doesNotMatch(html,/[\u4e00-\u9fff]/,def.id);
      }
    }
    for(const entry of ['ticket','reveal','settlement']){
      const html=renderHandbook(rules,{...newHandbookState('hints'),filter:'flow',entry});
      assert.ok(html.includes(t('book.event.'+entry+'.title')));
      if(locale==='en')assert.doesNotMatch(html,/[\u4e00-\u9fff]/);
    }
  }}finally{setLocale('zh-CN',false);}
});

test('catalog filters and pagination retain server prices and support empty and egg entries',()=>{
  const selected=rules.catalog.find(i=>i.category==='easter_egg');
  const egg=renderHandbook(rules,{...newHandbookState(),category:'easter_egg',entry:selected.id});
  assert.ok(egg.includes('handbook-egg')&&egg.includes('17,500,000'));
  assert.ok(!egg.includes(t('book.qualityRange')));
  const search=renderHandbook(rules,{...newHandbookState(),search:rules.catalog[0].name,quality:'common',footprint:'1x1'});
  assert.ok(search.includes('1,234'));
  assert.equal((search.match(/data-action="book-entry:/g)||[]).length,1);
  const second=renderHandbook(rules,{...newHandbookState(),page:1});
  assert.ok(second.includes('book-entry:'+rules.catalog[6].id));
  assert.ok(!second.includes('book-entry:'+rules.catalog[0].id));
  const empty=renderHandbook(rules,{...newHandbookState(),search:'no-matching-item',detail:true});
  assert.ok(empty.includes(t('book.noMatch'))&&!empty.includes('has-detail'));
});

test('public reference preserves custom text, aliases and translations without displaying private payloads',()=>{
  setLocale('en',false);
  try{
    const custom={...rules,catalog:[{...rules.catalog[0],id:'custom',name:'<Custom name>',translations:{en:'<Translated item>'}}],cards:[{...rules.cards[0],name:'<Custom card>',description:'<Custom explanation>'}],public_hint_definitions:[{id:'my_alias',builtin:'quality_average'},{id:'external',command:['private-executable'],builtin:''}],self:{bid:123456789,reason:'PRIVATE_REASON'},intelligence:{p1:'PRIVATE_INTEL'}};
    const item=renderHandbook(custom,newHandbookState());
    assert.ok(item.includes('&lt;Translated item&gt;')&&!item.includes('<Translated item>'));
    assert.equal(itemName({name:'Original host name'}),'Original host name');
    const card=renderHandbook(custom,newHandbookState('cards'));
    assert.ok(card.includes('&lt;Custom card&gt;')&&card.includes('&lt;Custom explanation&gt;'));
  const canonicalSearch=renderHandbook(rules,{...newHandbookState('cards'),search:'终极审计'});
  assert.ok(canonicalSearch.includes('book-entry:audit'));
    const hints=renderHandbook(custom,{...newHandbookState('hints'),entry:'external'});
    assert.ok(hints.includes('book-entry:my_alias')&&hints.includes(t('book.hint.quality_average.title')));
    for(const html of [item,card,hints])assert.doesNotMatch(html,/PRIVATE_REASON|PRIVATE_INTEL|123456789|private-executable|\{\}/);
  }finally{setLocale('zh-CN',false);}
});

test('specialty summaries distinguish sampled averages, remembered samples and complete revelations',()=>{
  const sampled=specialties.find(s=>s.id==='category').steps[1];
  assert.equal(effectText(sampled.effect,sampled),t('book.effect.sampleAverage',{count:3}));
  const full=specialties.find(s=>s.id==='electronics').steps[2];
  assert.ok(effectText(full.effect,full).includes(t('book.field.identity')));
  const html=renderHandbook(rules,{...newHandbookState('specialties'),entry:'small'});
  assert.ok(html.includes(t('book.remember'))&&html.includes(t('book.fromSample')));
  const filtered=renderHandbook(rules,{...newHandbookState('cards'),filter:'aggregate',search:'终极审计'});
  assert.ok(filtered.includes('book-entry:audit')&&!filtered.includes('book-entry:largest_price'));
});
