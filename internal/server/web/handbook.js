import {t,money,getLocale,localizeCard,localizeName,localizeItemName as itemName,catalogs} from './i18n.js';
import {esc,icon,button} from './html.js';
import {itemArtwork} from './item-art.js';

const sections=['items','cards','hints','specialties','boxes'];
const tr=(key,args)=>esc(t(key,args));
const list=values=>new Intl.ListFormat(getLocale(),{style:'short',type:'conjunction'}).format(values);
const quality=q=>catalogs[getLocale()][q]?t(q):q;
const name=(kind,id,rules)=>localizeName(kind,(rules[kind==='category'?'categories':'specialties']||[]).find(x=>x.id===id))||t('unknownSelection');
export {itemName};
export const newHandbookState=(section='items')=>({section:sections.includes(section)?section:'items',entry:null,search:'',category:'all',quality:'all',footprint:'all',filter:'all',page:0,detail:false});
export const hostReference=data=>({...data.game,qualities:data.qualities,catalog:data.catalog,cards:data.cards.cards,categories:data.categories,warehouses:data.warehouses,specialties:data.specialties,public_hint_definitions:data.public_hint_definitions});

const pill=content=>`<span class="handbook-pill">${content}</span>`;
const fact=(title,value)=>`<div><dt>${tr(title)}</dt><dd>${value}</dd></div>`;
const search=book=>`<input type="search" data-control="book-search" value="${esc(book.search)}" placeholder="${tr('book.search.'+book.section)}" aria-label="${tr('book.search.'+book.section)}">`;
const option=(id,label,current)=>`<option value="${esc(id)}" ${id===current?'selected':''}>${esc(label)}</option>`;
const select=(key,title,value,options)=>`<label class="handbook-select"><span>${tr(title)}</span><select data-control="book-${key}" aria-label="${tr(title)}">${options.map(([id,label])=>option(id,label,value)).join('')}</select></label>`;
const filterButtons=(options,value)=>`<div class="handbook-filters" role="group" aria-label="${tr('book.filters')}">${options.map(([id,label])=>button(esc(label),'book-filter:'+id,value===id?'selected':'',`aria-pressed="${value===id}"`)).join('')}</div>`;
const entry=(id,title,subtitle,selected,extra='')=>button(`<span class="handbook-entry-copy"><strong>${esc(title)}</strong>${subtitle?`<small>${esc(subtitle)}</small>`:''}</span>${extra}`,'book-entry:'+id,'handbook-entry '+(selected?'selected':''),`aria-pressed="${selected}"`);
const detail=(content,selected)=>`<section class="handbook-detail panel" data-region="book-detail:${selected}"><div class="handbook-mobile-actions">${button(icon('arrow')+tr('book.backList'),'book-list','text-button handbook-back')}${button(icon('close'),'book-exit','icon-button',`aria-label="${tr('book.close')}"`)}</div>${content}</section>`;
const noMatch=()=>`<p class="empty">${tr('book.noMatch')}</p>`;
const hasSearch=(text,query)=>String(text).toLocaleLowerCase(getLocale()).includes(query.trim().toLocaleLowerCase(getLocale()));

export function effectText(effect,step={}){
  if(step.mode==='presence')return t('book.effect.presence');
  if(effect.type==='average')return t(effect.sample_size?'book.effect.sampleAverageArea':'book.effect.averageArea',{count:effect.sample_size});
  if(effect.type==='reveal'){
    const target=step.all?t('book.target.all'):t('book.target.'+(effect.selector||'random'),{count:effect.count||1});
    const fields=[t('book.field.shape'),...(effect.fields||[]).filter(f=>f!=='shape').map(f=>t('book.field.'+f))];
    return t('book.effect.reveal',{target,fields:list(fields)});
  }
  return t('book.effect.'+(effect.type==='count'&&effect.group_by?effect.group_by+'_count':effect.type));
}
function scope(effect,rules,specialty,step={}){
  const parts=[];
  const categories=effect.category?[effect.category]:(specialty?.categories||[]);
  if(categories.length)parts.push(list(categories.map(id=>name('category',id,rules))));
  const qualities=effect.quality?[effect.quality]:(effect.qualities||step.qualities||[]);
  if(qualities.length)parts.push(t('book.qualityScope',{qualities:list(qualities.map(quality))}));
  if(effect.area)parts.push(t('book.areaScope',{count:effect.area}));
  if(effect.region)parts.push(t('book.region.'+effect.region));
  if(specialty?.tag)parts.push(t(specialty.tag==='clock'?'book.clockScope':'book.tagScope',{tag:specialty.tag}));
  if(specialty?.id==='region')parts.push(t('book.regionScope'));
  return parts.length?parts.join(' · '):t('wholeCrate');
}
const cardGroup=card=>card.effect.type==='reveal'?'individual':'aggregate';
const resultKey=effect=>effect.type==='average'?(effect.sample_size?'sample_average_area':'average_area'):effect.group_by?effect.group_by+'_count':effect.type;
function cardSummary(card,rules){
  const effect=card.effect;
  const chosenScope=card.target?t('book.targetScope.'+card.target):'';
  if(effect.type==='reveal')return t('book.scopeEffect',{scope:[chosenScope,effect.category||effect.quality||effect.area||effect.region||effect.qualities?.length?scope(effect,rules):'',t('book.shortTarget.'+(card.target==='item'?'chosen':effect.selector||'random'),{count:effect.count||1})].filter(Boolean).join(' · '),effect:list(['shape',...(effect.fields||[]).filter(f=>f!=='shape')].map(f=>t('book.field.'+f)))});
  return t('book.scopeEffect',{scope:chosenScope||scope(effect,rules),effect:t('book.result.'+resultKey(effect))});
}
const specialtyGroup=spec=>spec.categories?.length||spec.tag?'category':spec.steps.some(s=>s.qualities?.length||s.effect.quality)?'quality':'geometry';

function items(rules,book){
  const all=rules.catalog||[];
  const filtered=all.filter(i=>(book.category==='all'||i.category===book.category)&&(book.quality==='all'||i.quality===book.quality)&&(book.footprint==='all'||`${i.width}x${i.height}`===book.footprint)&&hasSearch(itemName(i)+' '+i.name,book.search));
  const pages=Math.max(1,Math.ceil(filtered.length/6)),page=Math.min(book.page,pages-1),shown=filtered.slice(page*6,page*6+6);
  const current=shown.find(i=>i.id===book.entry)||shown[0];
  const footprints=[...new Set(all.map(i=>`${i.width}x${i.height}`))];
  const controls=`<div class="handbook-controls">${select('category','book.category',book.category,[['all',t('book.all')],...(rules.categories||[]).map(c=>[c.id,localizeName('category',c)])])}${select('quality','quality',book.quality,[['all',t('allQuality')],...(rules.qualities||[]).map(q=>[q.id,quality(q.id)])])}${select('footprint','book.footprint',book.footprint,[['all',t('book.all')],...footprints.map(f=>[f,f.replace('x',' × ')])])}${search(book)}${button(tr('book.clear'),'book-clear','text-button')}</div>`;
  const rows=shown.map(i=>`<tr class="${current?.id===i.id?'selected':''}"><th scope="row">${button(itemArtwork(i,all)+`<span>${esc(itemName(i))}</span>`,'book-entry:'+i.id,'text-button handbook-item-select',`aria-pressed="${current?.id===i.id}"`)}<small class="handbook-mobile-meta">${esc(quality(i.quality))} · ${i.width} × ${i.height}</small></th><td>${esc(quality(i.quality))}</td><td class="footprint">${i.width} × ${i.height}</td><td class="handbook-price">${money(i.price)}</td></tr>`).join('');
  const left=`<section class="handbook-list panel" data-region="book-items"><header><strong aria-live="polite">${tr('book.matches',{count:money(filtered.length)})}</strong></header><div class="handbook-item-table"><table><thead><tr>${['book.item','quality','book.footprint','book.priceCoins'].map(k=>`<th scope="col">${tr(k)}</th>`).join('')}</tr></thead><tbody>${rows}</tbody></table>${shown.length?'':noMatch()}</div><div class="handbook-pagination"><small>${tr('book.pagination',{page:page+1,pages})}</small>${button(tr('book.previous'),'book-page:'+(page-1),'paper-button',page===0?'disabled':'')}${button(tr('book.next'),'book-page:'+(page+1),'paper-button',page+1>=pages?'disabled':'')}</div></section>`;
  let right='';
  if(current){
    const q=(rules.qualities||[]).find(q=>q.id===current.quality),artwork=itemArtwork(current,all);
    right=detail(`<span class="eyebrow">${tr('book.catalogDetails')}</span><h2 tabindex="-1">${esc(itemName(current))}</h2><div class="handbook-pills">${pill(esc(name('category',current.category,rules)))}${pill(esc(quality(current.quality)))}</div>${artwork?`<div class="handbook-artwork">${artwork}</div>`:''}<div class="handbook-amount"><div class="coin-amount">${icon('coin')}<strong>${money(current.price)}</strong><span class="sr-only">${tr('book.priceCaption')}</span></div></div><dl>${fact('book.footprint',tr('book.cells',{width:current.width,height:current.height}))}${q?fact('book.qualityRange',money(q.min_price)+' – '+money(q.max_price)+' '+tr('coins')):''}${fact('book.qualityOdds',money(current.quality_appearance_probability*100,10)+'%')}${fact('book.appearanceOdds',money(current.appearance_probability*100,10)+'%')}</dl>`,current.id);
  }
  return {controls,body:`${left}${right}`,selected:!!current};
}
function cards(rules,book){
  const all=rules.cards||[],filtered=all.filter(c=>(book.filter==='all'||cardGroup(c)===book.filter)&&hasSearch(localizeCard(c).name+' '+localizeCard(c).description+' '+c.name+' '+c.description,book.search));
  const selected=filtered.find(c=>c.id===book.entry)||filtered[0];
  const controls=`<div class="handbook-controls">${filterButtons(['all','individual','aggregate'].map(id=>[id,t('book.cardFilter.'+id,{count:all.filter(c=>id==='all'||cardGroup(c)===id).length})]),book.filter)}${search(book)}</div>`;
  const left=`<section class="handbook-list panel" data-region="book-cards"><header><strong>${tr('book.cardsCount',{count:filtered.length})}</strong><small>${tr('book.systemPrice')}</small></header><div class="handbook-card-list">${filtered.map(c=>{const localized=localizeCard(c);return entry(c.id,localized.name,cardSummary(c,rules),selected?.id===c.id,`<span class="handbook-price">${money(c.price)}</span>`);}).join('')||noMatch()}</div></section>`;
  let right='';
  if(selected){const c=localizeCard(selected),fields=c.effect.type==='reveal'?['shape',...(c.effect.fields||[]).filter(f=>f!=='shape')].map(f=>t('book.field.'+f)):[...(c.target?[]:[scope(c.effect,rules)]),t('book.result.'+resultKey(c.effect))];right=detail(`<span class="eyebrow">${tr('book.cardDetails')}</span><h2 tabindex="-1">${esc(c.name)}</h2><div class="handbook-amount"><strong>${money(c.price)}</strong><small>${tr('book.cardPriceCaption')}</small></div><p class="handbook-description">${esc(c.description)}</p><div class="handbook-pills">${c.target?pill(tr('book.targetScope.'+c.target)):''}${fields.map(f=>pill(esc(f))).join('')}${c.min_round>1?pill(tr('book.availableRound',{round:c.min_round})):''}</div>`,selected.id);}
  return {controls,body:left+right,selected:!!selected};
}
const builtinHints=new Set(['random_shape','random_quality','quality_count','quality_presence','quality_largest_area','category_count','category_quality_count','region_quality_count','quality_total_area','quality_average_area']);
function hints(rules,book){
  const all=rules.public_hint_definitions||[];
  const flow=book.filter==='flow';
  const entries=flow?['ticket','reveal','settlement'].map(id=>({id,kind:id})):all.map((d,i)=>({id:d.id,kind:builtinHints.has(d.builtin)?d.builtin:'custom',number:i+1}));
  const selected=entries.find(h=>h.id===book.entry)||entries[0];
  const title=h=>t('book.'+(flow?'event.':'hint.')+h.kind+'.title',{number:h.number});
  const controls=`<div class="handbook-controls">${filterButtons([['all',t('book.publicHints',{count:all.length})],['flow',t('book.flowEvents')]],book.filter)}</div>`;
  const left=`<section class="handbook-list panel" data-region="book-hints:${flow}"><header><strong>${tr(flow?'book.flowEvents':'book.hintDirectory')}</strong></header><div class="handbook-hint-list">${entries.map(h=>entry(h.id,title(h),t('book.'+(flow?'event.':'hint.')+h.kind+'.summary'),selected?.id===h.id)).join('')||noMatch()}</div></section>`;
  let right='';
  if(selected){
    const base='book.'+(flow?'event.':'hint.')+selected.kind;
    right=detail(`<div class="handbook-detail-head"><h2 tabindex="-1">${esc(title(selected))}</h2>${pill(tr('book.example'))}</div><div class="handbook-example"><p>${tr(base+'.example',{fee:money(rules.ticket_fee??0)})}</p></div>${flow?'':`<dl>${fact('book.when',tr('book.hintWhen'))}</dl>`}`,selected.id);
  }
  return {controls,body:left+right,selected:!!selected};
}
function specialties(rules,book){
  const all=rules.specialties||[],filtered=all.filter(s=>book.filter==='all'||specialtyGroup(s)===book.filter),selected=filtered.find(s=>s.id===book.entry)||filtered[0];
  const controls=`<div class="handbook-controls">${filterButtons(['all','category','quality','geometry'].map(id=>[id,t('book.specialtyFilter.'+id,{count:all.length})]),book.filter)}</div>`;
  const left=`<section class="handbook-list panel" data-region="book-specialties"><header><strong>${tr('book.specialtiesCount',{count:filtered.length})}</strong></header><div class="handbook-specialty-grid">${filtered.map(s=>entry(s.id,localizeName('specialty',s),s.steps.map(step=>String(step.round).padStart(2,'0')).join(' · '),selected?.id===s.id)).join('')||noMatch()}</div></section>`;
  let right='';
  if(selected){
    const steps=selected.steps.map(step=>`<article class="handbook-step"><span aria-label="${tr('round',{round:step.round})}">${String(step.round).padStart(2,'0')}</span><div><strong>${esc(effectText(step.effect,step))}</strong><p>${esc(scope(step.effect,rules,selected,step))}</p>${step.effect.unknown_only?`<small>${tr('book.unknownOnly')}</small>`:''}${step.condition?`<small>${tr('book.condition.'+step.condition)}</small>`:''}${step.remember?`<small>${tr('book.remember')}</small>`:''}${step.from_sample?`<small>${tr('book.fromSample')}</small>`:''}</div></article>`).join('');
    right=detail(`<span class="eyebrow">${tr('book.specialtyDetails')}</span><h2 tabindex="-1">${esc(localizeName('specialty',selected))}</h2><div class="handbook-steps">${steps}</div>`,selected.id);
  }
  return {controls,body:left+right,selected:!!selected};
}
function boxes(rules,book){
  const categories=box=>(box.favored_categories||[]).map(id=>name('category',id,rules));
  const all=rules.warehouses||[],filtered=all.filter(box=>hasSearch([localizeName('warehouse',box),box.name,...categories(box),...(box.favored_categories||[]).map(id=>rules.categories?.find(c=>c.id===id)?.name||id)].join(' '),book.search));
  const selected=filtered.find(box=>box.id===book.entry)||filtered[0];
  const controls=`<div class="handbook-controls">${search(book)}</div>`;
  const left=`<section class="handbook-list panel" data-region="book-boxes"><header><strong>${tr('book.boxesCount',{count:filtered.length})}</strong></header><div class="handbook-box-list">${filtered.map(box=>entry(box.id,localizeName('warehouse',box),categories(box).join(' · ')||t('noCategoryPreference'),selected?.id===box.id)).join('')||noMatch()}</div></section>`;
  let right='';
  if(selected){
    const favored=selected.favored_categories||[],related=(rules.specialties||[]).filter(s=>s.categories?.some(id=>favored.includes(id)));
    right=detail(`<span class="eyebrow">${tr('book.boxDetails')}</span><h2 tabindex="-1">${esc(localizeName('warehouse',selected))}</h2><div class="handbook-pills">${(categories(selected).length?categories(selected):[t('noCategoryPreference')]).map(category=>pill(esc(category))).join('')}</div><p class="handbook-description">${tr(favored.length?'book.boxBias':'book.boxMixed')}</p><dl class="handbook-box-facts">${fact('book.boxSelection',tr('book.boxSelectionRule'))}${fact('book.boxShared',tr('book.boxSharedRule'))}${related.length?fact('book.boxSpecialties',esc(list(related.map(s=>localizeName('specialty',s))))):''}</dl>`,selected.id);
  }
  return {controls,body:left+right,selected:!!selected};
}
export function renderHandbook(rules,book,{pending=false,failed=false,roomName=''}={}){
  if(rules)rules={...rules,catalog:(rules.catalog||[]).filter(item=>item.category!=='easter_egg'&&item.quality!=='easter_egg'),categories:(rules.categories||[]).filter(category=>category.id!=='easter_egg'),qualities:(rules.qualities||[]).filter(quality=>quality.id!=='easter_egg')};
  const section=sections.includes(book.section)?book.section:'items';
  const counts={items:rules?.catalog?.length||0,cards:rules?.cards?.length||0,hints:rules?.public_hint_definitions?.length||0,specialties:rules?.specialties?.length||0,boxes:rules?.warehouses?.length||0};
  const nav=`<nav class="handbook-nav" aria-label="${tr('handbook')}">${sections.map(id=>button(`<strong>${tr('book.section.'+id)}</strong><small>${id==='hints'?tr('book.publicHints',{count:counts[id]}):tr('book.'+id+'Count',{count:counts[id]})}</small>`,'book-section:'+id,section===id?'selected':'',section===id?'aria-current="page"':'')).join('')}</nav>`;
  const head=`<header class="handbook-heading"><div><span class="eyebrow">${tr('handbook')} / ${String(sections.indexOf(section)+1).padStart(2,'0')}</span><h1>${tr('book.section.'+section)}</h1></div>${button(icon('arrow')+tr(roomName?'book.backRoom':'book.backLobby'),'book-exit','paper-button')}</header>`;
  let content;
  if(pending||!rules)content=`<div class="handbook-status" role="status"><p>${tr(failed?'book.loadError':'loading')}</p>${failed?button(tr('book.retry'),'book-retry','paper-button'):''}</div>`;
  else{
    const view=({items,cards,hints,specialties,boxes})[section](rules,book);
    content=`${view.controls}<div class="handbook-reader ${book.detail&&view.selected?'has-detail':''}" data-region="book-reader:${section}">${view.body}</div>`;
  }
  return `<section class="handbook" data-region="handbook">${nav}<div class="handbook-main">${head}${content}</div></section>`;
}
