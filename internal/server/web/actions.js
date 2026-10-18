import {t,money,getLocale,localizeCard,localizeName,localizeItemName,catalogs} from './i18n.js';
import {esc,icon,button} from './html.js';

const tr=(key,args)=>esc(t(key,args));
const readActions=new Set(['handbook','state','catalog','config','cards','hints','inventory','me','replay','watch']);
const iconFor=e=>e.action==='handbook'?'book':['bid','ticket','buy','card'].includes(e.action)?'coin':e.action==='withdraw'?'arrow':'scan';
const time=e=>e.at?new Intl.DateTimeFormat(getLocale(),{hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false}).format(new Date(e.at)):'';
export const playerActivities=(view,id)=>(view.activities||[]).filter(e=>!e.player_id||e.player_id===id);
export function activityTitle(e,view){
 if(e.action==='card'){const c=view.rules.cards?.find(c=>c.id===e.params?.card);return t('action_card')+(c?' · '+localizeCard(c).name:'');}
 if(e.action==='handbook')return t('action_handbook')+(e.params?.topic?' · '+(catalogs[getLocale()]['topic_'+e.params.topic]?t('topic_'+e.params.topic):t('unknownSelection')):'');
 if(e.action==='specialty')return t('action_specialty')+(e.result?.title?' · '+(localizeName('specialty',view.rules.specialties?.find(s=>s.name===e.result.title))||e.result.title):'');
 return catalogs[getLocale()]['action_'+e.action]?t('action_'+e.action):t('actionOther');
}
export function intelligenceTitle(hint,view){
 const data=hint.data||{},title=data.translations?.[getLocale()]?.title||data.title;
 if(hint.source==='specialty'){
  const specialty=view.rules.specialties?.find(s=>s.id===hint.kind);
  return !title||title===specialty?.name?localizeName('specialty',specialty)||t('specialtyIntel'):title;
 }
 const definition=view.rules.cards?.find(c=>c.id===hint.source);
 return title||(definition?localizeCard(definition).name:t('privateSkills'));
}
export function intelligenceSummary(data,view){
 if(!data)return '';
 if(data.items)return t('intelSample',{count:data.items.length});
 if(data.average_price!==undefined)return t(data.sample_count!==undefined?'intelSampleAverage':'intelAverage',{count:data.sample_count,value:data.average_price===null?t('noCandidates'):money(data.average_price)});
 if(data.average_area!==undefined)return t('intelAverageArea',{value:data.average_area===null?t('noCandidates'):money(data.average_area,2)});
 if(data.present!==undefined)return t(data.present?'intelPresent':'intelAbsent');
 if(data.total_value!==undefined)return t('intelValue',{value:money(data.total_value)});
 if(data.counts)return Object.entries(data.counts).map(([key,value])=>`${localizeName('category',view.rules.categories?.find(c=>c.id===key))||(catalogs[getLocale()][key]?t(key):t('unknownSelection'))} ${money(value)}`).join(' · ');
 if(data.area!==undefined)return t('intelArea',{count:data.area});
 if(data.count!==undefined)return t('intelCount',{count:data.count});
 if(data.item_id)return [localizeItemName(data,view.rules.catalog)||data.item_id,data.quality?t(data.quality):null,data.price!==undefined?money(data.price):null].filter(Boolean).join(' · ');
 return data.translations?.[getLocale()]?.message||data.message||'';
}
function grouped(events){
 const rows=[];for(const event of events){const last=rows.at(-1);if(last&&readActions.has(event.action)&&event.status==='success'&&last.event.action===event.action&&last.event.round===event.round&&last.event.status===event.status){last.events.push(event)}else rows.push({event,events:[event]})}return rows;
}
function summary(e,view){if(e.status==='rejected')return t('actionRejected');if(['specialty','public_hint'].includes(e.action))return intelligenceSummary(e.result,view);if(e.cost)return t('actionCost',{value:money(e.cost)});if(e.action==='card')return intelligenceSummary(e.result?.data,view);if(e.action==='bid')return t('activity_'+e.status);if(e.params?.amount!==undefined)return money(e.params.amount);return t('source_'+e.source);}
const selectedActivityRound=(view,ui)=>Math.min(ui.actionRound??(ui.cursor===null||ui.cursor===undefined?view.round:ui.cursor),view.round);
const matchesFilter=(e,filter)=>filter==='all'||filter==='decisions'&&['bid','card','withdraw'].includes(e.action)||filter==='intel'&&['card','specialty','public_hint'].includes(e.action)||filter==='tools'&&readActions.has(e.action);
export function actionRows(view,id,round,filter='all',selected=null){
 const events=playerActivities(view,id).filter(e=>e.round===round&&matchesFilter(e,filter));
 return grouped(events).map(({event:e,events:all})=>{
  const failed=e.status==='rejected';const content=`<span class="action-marker">${icon(iconFor(e))}</span><span class="action-copy"><strong>${esc(activityTitle(e,view))}${all.length>1?` <small>× ${all.length}</small>`:''}</strong><small>${esc(summary(e,view))}</small>${e.action==='bid'&&e.status==='success'?`<b class="action-bid-value">${money(e.params.amount)}</b>`:''}${e.reason?`<span class="action-reason">${esc(e.reason)}</span>`:''}</span><time datetime="${esc(e.at)}">${esc(time(e))}</time>`;
  if(all.length>1)return `<details class="action-group" data-region="activity-group:${e.id}"><summary>${content}</summary><div>${all.map(item=>button(`<span>#${item.id} · ${esc(time(item))}</span><span>${tr('viewCall')}</span>`,'action-detail:'+item.id,'action-group-item')).join('')}</div></details>`;
  return button(content,'action-detail:'+e.id,`action-row ${e.action==='bid'?'action-bid':''} ${failed?'action-failed':''} ${selected===e.id?'selected':''}`);
 }).join('')||`<p class="peek-empty">${tr('noActivities')}</p>`;
}
export function activityTabs(view,ui){
 const round=selectedActivityRound(view,ui);
 return `<nav class="activity-rounds" aria-label="${tr('activityRounds')}">${Array.from({length:6},(_,n)=>button(n?String(n).padStart(2,'0'):tr('preparation'),'action-round:'+n,n===round?'selected':'',`${n>view.round?'disabled':''} aria-pressed="${n===round}"`)).join('')}</nav>`;
}
function activityListHead(view,ui,round){
 const count=playerActivities(view,ui.focusPlayer).filter(e=>e.round===round).length;
 return `<div class="activity-list-head"><span>${round?tr('round',{round}):tr('preparation')} · ${tr('activityCount',{count})}</span><label><span class="sr-only">${tr('filterActions')}</span><select data-control="action-filter" aria-label="${tr('filterActions')}">${['all','decisions','intel','tools'].map(value=>`<option value="${value}" ${ui.actionFilter===value?'selected':''}>${tr('actions_'+value)}</option>`).join('')}</select></label></div>`;
}
export function activityList(view,ui){
 const round=selectedActivityRound(view,ui);
 return `${activityTabs(view,ui)}${activityListHead(view,ui,round)}<div class="activity-feed" data-region="activities:${esc(ui.focusPlayer)}:${round}">${actionRows(view,ui.focusPlayer,round,ui.actionFilter||'all')}</div>`;
}
function actionFacts(e,view,card){
 const p=e.params||{},rows=[],add=(key,value)=>{if(value!==undefined&&value!=='')rows.push(`<div><dt>${tr(key)}</dt><dd>${esc(value)}</dd></div>`);};
 const name=(kind,id,list)=>localizeName(kind,list?.find(d=>d.id===id))||t('unknownSelection');
 const cardName=id=>{const d=view.rules.cards?.find(c=>c.id===id);return d?localizeCard(d).name:t('unknownSelection');};
 const round=Number.isInteger(p.round)&&p.round>=1&&p.round<=5?p.round:e.round;
 add('actionStage',round?t('round',{round}):t(e.action==='ticket'?'opening':'preparation'));
 if(e.action==='bid')add('bidFormat',t('sealedOffer'));
 if(p.card)add(e.action==='buy'?'purchasedCard':'usedCard',cardName(p.card));
 if(p.quantity!==undefined)add('purchaseQuantity',t('cardQuantity',{count:p.quantity}));
 if(p.specialty)add('selectedSpecialty',name('specialty',p.specialty,view.rules.specialties));
 if(p.category)add('selectedCategory',catalogs[getLocale()]['region_'+p.category]?t('region_'+p.category):name('category',p.category,view.rules.categories));
 if(e.action==='catalog'){
  if(p.name)add('catalogName',p.name);
  if(p.size)add('catalogSize',p.size.replace(/[xX]/g,' × '));
  if(p.quality){const q=view.rules.qualities?.find(q=>q.name===p.quality||q.id===p.quality),key=q?.id||p.quality;add('catalogQuality',catalogs[getLocale()][key]?t(key):q?.name||p.quality);}
 }
 if(Array.isArray(p.cards))add('carriedCards',p.cards.length?p.cards.map(cardName).join(' · '):t('noCarriedCards'));
 if(p.all===true||Array.isArray(p.ids)&&p.ids.length)add('soldSelection',p.all===true?t('allStoredItems'):t('selectedItemCount',{count:p.ids.length}));
 if(p.players!==undefined)add('players',money(p.players));
 if(p.warehouse)add('warehouseType',name('warehouse',p.warehouse,view.rules.warehouses));
 if(p.ticket_fee!==undefined)add('ticketFee',t('coinValue',{value:money(p.ticket_fee)}));
 if(p.topic)add('handbookTopic',catalogs[getLocale()]['topic_'+p.topic]?t('topic_'+p.topic):t('unknownSelection'));
 if(p.lang)add('language',t(p.lang==='en'?'languageEnglish':p.lang==='zh-CN'?'languageChinese':'unknownSelection'));
 if(p.frame!==undefined&&Number.isInteger(Number(p.frame)))add('replayPosition',Number(p.frame)?t('round',{round:Number(p.frame)}):t('opening'));
 const effect=card?{...card.effect,...(card.target&&p.target?{[card.target==='item'?'item_id':card.target]:p.target}:{})}:null;
 if(effect){
  const filters=[];
  const qualities=effect.quality?[effect.quality]:effect.qualities||[];
  if(qualities.length)filters.push(t('book.qualityScope',{qualities:qualities.map(q=>catalogs[getLocale()][q]?t(q):t('unknownSelection')).join(' + ')}));
  if(effect.category)filters.push(name('category',effect.category,view.rules.categories));
  if(effect.area)filters.push(t('book.areaScope',{count:effect.area}));
  if(effect.item_id)filters.push(t('chosenItem',{id:effect.item_id}));
  if(effect.region)filters.push(catalogs[getLocale()]['book.region.'+effect.region]?t('book.region.'+effect.region):effect.region);
  const target=effect.selector==='largest'?t('largestItem'):effect.selector==='highest'||effect.selector==='smallest'?t('book.shortTarget.'+effect.selector,{count:effect.count||1}):effect.selector==='random'?t('randomItemCount',{count:effect.count||1}):'';
  const scope=[...filters,target].filter(Boolean).join(' · ')||t('wholeCrate');
  add('viewScope',scope);
  if(effect.type==='count')add('intelContent',t(effect.group_by==='quality'?'qualityCounts':effect.group_by==='category'?'categoryCounts':'totalItemCount'));
 }
 if(e.action==='bid'&&e.status==='success')add('actionState',t(view.history?.some(h=>h.round===e.round)?'offerRevealed':'offerLocked'));
 add('recordSource',t('source_'+e.source));
 if(e.at)add('recordTime',time(e));
 return rows.join('');
}
function actionOutcome(e,view){
 if(e.status==='rejected')return t('actionRejected');
 if(['specialty','public_hint'].includes(e.action))return intelligenceSummary(e.result,view);
 if(e.action==='card')return intelligenceSummary(e.result?.data,view)||t('cardApplied');
 if(e.action==='catalog'&&e.result?.templates!==undefined)return t('catalogMatches',{count:e.result.templates});
 if(e.action==='bid'||e.action==='withdraw')return '';
 return '';
}
export function actionDetail(e,view){
 if(!e)return `<p class="peek-empty">${tr('selectAction')}</p>`;
 const definition=e.action==='card'?view.rules.cards?.find(c=>c.id===e.params?.card):null,card=definition?localizeCard(definition):null,success=e.status==='success';
 const status=success&&e.action==='ticket'?'ticketPaid':success&&e.action==='card'?'cardApplied':'activity_'+e.status;
 const amount=e.action==='bid'?e.params?.amount:success&&['ticket','card'].includes(e.action)?e.cost??0:undefined;
 const amountLabel=e.action==='bid'?'offerAmount':e.action==='card'?'cardCost':'actionSpend';
 const result=actionOutcome(e,view),outcome=result?`<section class="action-outcome"><strong>${esc(result)}</strong></section>`:'';
 return `<article class="action-detail"><header><h3>${esc(activityTitle(e,view))}</h3><span class="${e.status==='rejected'?'negative':'positive'}">${tr(status)}</span></header>${amount!==undefined?`<div class="action-amount"><small>${tr(amountLabel)} · ${tr('coins')}</small><strong class="action-detail-amount">${money(amount)}</strong></div>`:''}${card?`<div class="action-card-help"><p>${esc(card.description)}</p>${button(tr('cardHelp',{name:card.name}),'card:'+card.id,'text-button')}</div>`:''}${e.reason?`<section class="action-reason-block"><small>${tr(e.action==='withdraw'?'withdrawReason':'submittedReason')}</small><p>${esc(e.reason)}</p></section>`:''}${e.action==='ticket'?outcome:''}<h4>${tr('actionDetails')}</h4><dl>${actionFacts(e,view,card)}</dl>${e.action==='ticket'?'':outcome}</article>`;
}
export function fullActivity(view,ui){
 if(!ui.observer)return `<p class="peek-empty">${tr('activityRestrictedTitle')}</p>`;
 const round=selectedActivityRound(view,ui),list=playerActivities(view,ui.focusPlayer).filter(e=>e.round===round&&matchesFilter(e,ui.actionFilter||'all')),selected=list.find(e=>e.id===ui.actionID);
 const player=view.players.find(p=>p.id===ui.focusPlayer),specialty=view.rules.specialties?.find(s=>s.id===player?.specialty);
 return `<p class="activity-context">${esc(player?.name||'')} · ${esc(localizeName('specialty',specialty))} · ${tr('authorizedObserver')}</p>${activityTabs(view,ui)}<div class="full-activity-layout"><div class="full-activity-list">${activityListHead(view,ui,round)}${actionRows(view,ui.focusPlayer,round,ui.actionFilter||'all',selected?.id)}</div><div class="full-activity-detail">${actionDetail(selected,view)}</div></div>`;
}
