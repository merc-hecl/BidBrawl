import {t, money, signed, localizeCard, getLocale} from './i18n.js';
import {knownItems, observation} from './model.js';
import {esc,icon,button} from './html.js';
import {roomView,avatarGallery} from './room-ui.js';
export {esc} from './html.js';

const tr = (key, values) => esc(t(key, values));
const label = value => String(value).padStart(2, '0');
const quality = value => ['common','uncommon','rare','epic','legendary'].includes(value) ? tr(value) : esc(value || t('unrevealed'));
const panel = (title, content, aside = '', cls = '') => `<section data-region="${title}" class="panel ${cls}"><div class="panel-head"><h2>${tr(title)}</h2>${aside}</div>${content}</section>`;
const empty = key => `<p class="empty compact">${tr(key)}</p>`;
const details = pairs => `<dl>${pairs.map(([key,value]) => `<div><dt>${tr(key)}</dt><dd>${value}</dd></div>`).join('')}</dl>`;
export const maxFrame = source => source.frames.length-1;
export function status(source) {
  return source.phase === 'lobby' ? t('lobby') : source.phase === 'settled' ? t('settled') :
    t('roundProgress',{round:source.round,total:source.rules.rounds,count:source.players.filter(p=>p.submitted).length,players:source.players.length});
}
export const roomGroup = room => room?.phase === 'settled' ? 'completed' : 'active';
export function roomButtons(rooms, selected, group) {
  return rooms.filter(room => roomGroup(room) === group).map(room => button(`<span class="room-dot ${esc(room.phase)}"></span><span>${esc(room.name)}<small>${tr('roomSeats',
    {count:room.players,total:room.player_count,phase:t({lobby:'phaseLobby',bidding:'phaseBidding',settled:'phaseSettled'}[room.phase])})}</small></span>`,
    'room:'+room.room_id, selected === room.room_id ? 'selected' : '', selected === room.room_id ? 'aria-current="true"' : '')).join('') || `<p class="side-empty">${tr(group === 'completed' ? 'noCompletedRooms' : 'noActiveRooms')}</p>`;
}
function bids(view) {
  return `<div class="table-scroll"><table class="bid-table"><thead><tr><th scope="col">${tr('agent')}</th>${Array.from({length:view.rules.rounds},(_,i)=>`<th scope="col">${label(i+1)}${i===view.settlement?.round-1?`<small>${tr('finalRound')}</small>`:''}</th>`).join('')}</tr></thead><tbody>${view.players.map((player,i)=>`<tr><th scope="row">${button(`<i class="agent-color-${i}"></i>${esc(player.name)}`,'agent:'+player.id,'text-button agent-name')}</th>${Array.from({length:view.rules.rounds},(_,r)=>{
    const record=view.history.find(h=>h.round===r+1),bid=record?.bids.find(b=>b.player_id===player.id);
    const high=bid && bid.amount===Math.max(...record.bids.map(b=>b.amount));
    return `<td class="${high?'high':''}">${bid?money(bid.amount):r+1===view.round && view.phase==='bidding'?`<span class="sealed-bid">${tr('sealed')}</span>`:'—'}</td>`;
  }).join('')}</tr>`).join('')}</tbody></table>${view.players.length?'':empty('noBids')}</div>`;
}
function hints(view) {
  return `<section class="intel-panel"><div class="intel-list">${view.public_hints.map(hint=>{
    const text=hint.data.translations?.[getLocale()]||hint.data;
    return `<article><div class="intel-marker">${icon('scan')}</div><div class="intel-copy"><span class="eyebrow">${tr('round',{round:hint.round})}</span><h3>${esc(text.title)}</h3><p>${esc(text.message)}</p></div></article>`;
  }).join('')||empty('noHints')}</div></section>`;
}
function settlement(view) {
  if(!view.settlement)return `<div class="pending-settlement"><span>${tr('pending')}</span></div>`;
  const r=view.settlement,winner=view.players.find(p=>p.id===r.winner_id);
  return `<div class="settlement"><div><p class="eyebrow">${tr('finalOutcome',{round:r.round,method:t(r.method==='instant'?'instantSale':r.tied_players.length>1?'tieWin':'highestWins')})}</p><h3>${tr('winner',{name:winner?.name||r.winner_id})}</h3></div>${details([['paid',money(r.paid)],['totalValue',money(r.total_value)],['profit',`<span class="${r.profit>=0?'positive':'negative'}">${signed(r.profit)}</span>`]])}</div>`;
}
function inventory(view,ui) {
  if(!view.settlement)return '';
  const items=view.settlement.items.filter(item=>(ui.quality==='all'||item.quality===ui.quality)&&item.id.toLowerCase().includes(ui.search.toLowerCase()));
  return `<section class="panel" data-region="inventory"><div class="inventory-controls"><label>${tr('quality')} <select data-control="quality" aria-label="${tr('quality')}"><option value="all">${tr('allQuality')}</option>${['common','uncommon','rare','epic','legendary'].map(q=>`<option value="${q}" ${ui.quality===q?'selected':''}>${tr(q)}</option>`).join('')}</select></label><input data-control="search" aria-label="${tr('searchItem')}" placeholder="${tr('searchItem')}" value="${esc(ui.search)}"><small>${tr('itemCountRatio',{count:items.length,total:view.settlement.items.length})}</small></div><div class="inventory-table table-scroll"><table><thead><tr>${['id','quality','area','bounds','value'].map(key=>`<th>${tr(key)}</th>`).join('')}</tr></thead><tbody>${items.map(item=>`<tr><td>${button(esc(item.id),'item:'+item.id,'text-button')}</td><td><i class="quality-${esc(item.quality)}"></i>${quality(item.quality)}</td><td>${item.bottom_right.x-item.top_left.x} × ${item.bottom_right.y-item.top_left.y}</td><td class="mono">(${item.top_left.x}, ${item.top_left.y}) → (${item.bottom_right.x}, ${item.bottom_right.y})</td><td>${money(item.price)}</td></tr>`).join('')}</tbody></table>${items.length?'':empty('noItems')}</div></section>`;
}
function stats(results,ui) {
  const rows=results.agents,profit=results.profit;
  const rate=value=>value===null?'—':money(value,2)+'%';
  const sortControl=`<label>${tr('sort')} <select data-control="sort" aria-label="${tr('statsSort')}">${[['profit','cumulativeProfit'],['rate','profitRate'],['average','averageProfit'],['max_profit','maxProfit'],['max_loss','maxLoss']].map(([value,key])=>`<option value="${value}" ${ui.sort===value?'selected':''}>${tr(key)}</option>`).join('')}</select></label>`;
  return `<div class="metrics">${[['sampleCount',results.games],['cumulativeProfit',results.games?signed(profit):'—'],['profitRate',rate(results.rate)]].map(([key,value])=>`<article><p>${tr(key)}</p><strong>${value}</strong></article>`).join('')}</div>${panel('scorecard',rows.length?`<div class="table-scroll"><table class="stats-table"><thead><tr>${['agent','participation','profitRate','averageProfit','maxProfit','maxLoss','cumulativeProfit'].map(key=>`<th>${tr(key)}</th>`).join('')}</tr></thead><tbody>${rows.map(row=>`<tr><th scope="row">${button(esc(row.name),'stats-agent:'+row.id,'text-button')}</th><td>${row.games}</td><td>${rate(row.rate)}</td><td class="${row.average>=0?'positive':'negative'}">${signed(row.average,2)}</td><td>${signed(row.max_profit)}</td><td class="${row.max_loss<0?'negative':''}">${signed(row.max_loss)}</td><td class="${row.profit>=0?'positive':'negative'}">${signed(row.profit)}</td></tr>`).join('')}</tbody></table></div>`:empty('noResults'),sortControl)}`;
}

export function roomDirectory(rooms,ui){
  return `<div class="page-heading"><h1>${tr('rooms')}</h1>${ui.manage?button(tr('createRoom'),'create-room','quiet'):''}</div><div class="room-directory">${rooms.map(room=>`<article class="panel room-tile"><span class="eyebrow">${tr(room.phase==='lobby'?'waiting':room.phase==='settled'?'settled':'bidding')}</span><h2>${esc(room.name)}</h2><div class="room-tile-footer"><span>${room.players} / ${room.player_count} ${tr('players')}</span>${button(tr('enterRoom'),'room:'+room.room_id,'quiet')}</div></article>`).join('')||`<div class="empty-room">${icon('brand')}<h2>${tr('noRooms')}</h2>${ui.manage?button(tr('createRoom'),'create-room','quiet'):''}</div>`}</div>`;
}
export function renderContent(source,results,ui) {
  if(ui.page==='stats')return `<div class="page-heading"><h1>${tr('agentStats')}</h1></div>`+stats(results,ui);
  if(!source)return empty('loading');
  const view=observation(source,ui.cursor);
  const state=ui.cursor!==null?(ui.cursor===0?tr('opening'):view.settlement?tr('settled'):tr('round',{round:ui.cursor})):esc(status(view));
  return roomView(source,view,ui,state);
}
export function dialogContent(kind,id,source,results,ui) {
  const view=source?observation(source,ui.cursor):null;
  if(kind==='room-access')return {title:t('createRoom'),content:`<p class="room-access-message">${tr('createRoomAccess')}</p>`};
  if(kind==='create-room')return {title:t('createRoom'),content:`<form id="create-room-form" class="room-form"><label>${tr('roomName')}<input name="name" maxlength="32" required autocomplete="off"></label><label>${tr('players')}<select name="players">${[2,3,4,5,6,7].map(count=>`<option value="${count}" ${count===4?'selected':''}>${count}</option>`).join('')}</select></label><p id="create-room-error" class="negative" hidden>${tr('createRoomError')}</p><button type="submit" class="quiet">${tr('createRoom')}</button></form>`};
  if(kind==='invite-room')return {title:t('invitePlayers'),content:`<div class="invite-content"><label for="invite-link">${tr('inviteLink')}</label><div class="invite-copy"><input id="invite-link" readonly value="${esc(id)}">${button(tr('copyInvite'),'copy-invite','quiet')}</div><label>${tr('joinCommand')}</label><code class="join-command">bidbrawl join -invite "${esc(id)}"</code></div>`};
  if(kind==='room-error')return {title:t('roomActionError'),content:''};
  if(kind==='avatars')return {title:t('avatarGallery'),content:avatarGallery()};
  if(kind==='hints')return view?{title:t('hints'),content:hints(view)}:null;
  if(kind==='bids')return view?{title:t('bids'),content:bids(view)}:null;
  if(kind==='inventory')return view?.settlement?{title:t('inventory'),content:settlement(view)+inventory(view,ui)}:null;
  if(kind==='card'){
    const definition=source?.rules.cards.find(card=>card.id===id);
    if(!ui.observer || !definition)return null;
    const card=localizeCard(definition);
    return {title:card.name,content:`<p class="card-description">${esc(card.description)}</p>`};
  }
  if(kind==='rules'){
    if(!source)return {title:t('rules'),content:empty('loading')};
    const rules=source.rules;
    const pairs=[['players',tr('playersRule',{count:rules.player_count})],['rounds',tr('roundsRule',{count:rules.rounds})],['sale',(rules.early_settlement_multipliers||[]).some(Boolean)?tr('instantRule'):tr('saleRule',{round:rules.rounds})],['ties',tr('tieRule')],['publicClues',tr('publicCluesRule')],['bidPublication',tr('bidPublicationRule')],['privateSkills',tr('privateRule')],['box',tr('dimensions',{width:rules.box_width,height:rules.box_height})],['coordinates',tr('coordinatesRule')],['profit',tr('profitRule')],['otherReturn','0']];
    const geometry=rules.game_config?.items;
    if(geometry)pairs.push(['itemMax',`${geometry.max_width} × ${geometry.max_height}`]);
    pairs.push(['cardLimit',tr('cardsCount',{count:5})]);
    return {title:t('rules'),content:details(pairs).replace('<dl>','<dl class="rule-grid">')+`<details class="card-rules"><summary>${tr('cardCatalog')}</summary>${(rules.cards||[]).map(localizeCard).map(card=>`<article><h3>${esc(card.name)}</h3><p>${esc(card.description)}</p></article>`).join('')}</details>`};
  }
  if(kind==='item'){
    const item=view && knownItems(view).find(item=>item.id===id);if(!item)return null;
    return {title:item.id,content:details([['quality',quality(item.quality)],['value',item.price===undefined?tr('unrevealed'):money(item.price)],['area',`${item.bottom_right.x-item.top_left.x} × ${item.bottom_right.y-item.top_left.y}`],['bounds',`[(${item.top_left.x}, ${item.top_left.y}), (${item.bottom_right.x}, ${item.bottom_right.y}))`]])};
  }
  const seated=kind==='agent'?view?.players.find(player=>player.id===id):null;
  const record=ui.agentProfile || results.agents.find(agent=>agent.id===(kind==='stats-agent'?id:seated?.agent_id));
  const player=seated || record;if(!player)return null;
  const accountId=(player.agent_id||player.id).replace(/^account:/,'');
  const metrics=record?[['participation',money(record.games)],['profitRate',record.rate===null?'—':money(record.rate,2)+'%'],['averageProfit',signed(record.average,2)],['cumulativeProfit',signed(record.profit)],['maxProfit',signed(record.max_profit)],['maxLoss',signed(record.max_loss)]]:[];
  return {title:player.name,content:`<section class="agent-profile"><div class="agent-identity"><span>${tr('id')}</span><code>${esc(accountId)}</code></div><div class="agent-metrics">${metrics.map(([key,value])=>`<article class="${key==='profitRate'||key==='cumulativeProfit'?'featured':''}"><span>${tr(key)}</span><strong class="${/^[−-]/.test(value)?'negative':''}">${value}</strong></article>`).join('')}</div></section>`};
}
