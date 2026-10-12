import {t, money, signed, localizeCard} from './i18n.js';
import {knownItems, observation} from './model.js';

export const esc = value => String(value).replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
const tr = (key, values) => esc(t(key, values));
const label = value => String(value).padStart(2, '0');
const icon = name => `<span class="icon icon-${name}" aria-hidden="true"></span>`;
const colors = ['#526b7a','#b18951','#578578','#8b6660','#6f7c55','#66878c','#96907e'];
const itemColors = {common:'#8f9e98',uncommon:'#729e89',rare:'#668da4',epic:'#b18b72',legendary:'#c1a15e'};
const quality = value => ['common','uncommon','rare','epic','legendary'].includes(value) ? tr(value) : esc(value || t('unrevealed'));
const button = (text, action, cls = '', extra = '') => `<button class="${cls}" data-action="${esc(action)}" ${extra}>${text}</button>`;
const panel = (title, content, aside = '', cls = '') => `<section data-region="${title}" class="panel ${cls}"><div class="panel-head"><h2>${tr(title)}</h2>${aside}</div>${content}</section>`;
const empty = key => `<p class="empty compact">${tr(key)}</p>`;
const details = pairs => `<dl>${pairs.map(([key,value]) => `<div><dt>${tr(key)}</dt><dd>${value}</dd></div>`).join('')}</dl>`;
export const maxFrame = source => source.frames.length-1;
export function status(source) {
  return source.phase === 'lobby' ? t('lobby') : source.phase === 'settled' ? t('settled') :
    t('bidding',{round:source.round,total:source.rules.rounds,count:source.players.filter(p=>p.submitted).length,players:source.players.length});
}
export function roomButtons(rooms, selected) {
  return rooms.map(room => button(`<span class="room-dot ${esc(room.phase)}"></span><span>${esc(room.name)}<small>${tr('roomSeats',
    {count:room.players,total:room.player_count,phase:t({lobby:'phaseLobby',bidding:'phaseBidding',settled:'phaseSettled'}[room.phase])})}</small></span>`,
    'room:'+room.room_id, selected === room.room_id ? 'selected' : '', selected === room.room_id ? 'aria-current="true"' : '')).join('');
}
function timeline(source, view, cursor) {
  const finalFrame=source.settlement?source.history.length+1:source.rules.rounds+1;
  const selected=cursor??(source.settlement?finalFrame:source.history.length);
  const steps=[[t('opening'),0,false],...Array.from({length:source.rules.rounds},(_,i)=>[t('round',{round:i+1}),i+1,false]),[t('settlement'),finalFrame,true]];
  return `<section class="round-track" aria-label="${tr('timeline')}"><div class="round-heading"><span>${tr('rounds')}</span><small>${view.history.length} / ${source.rules.rounds}</small></div><div class="round-bottom"><div class="round-steps">${steps.map(([text,n,final])=>{
    const disabled=final?!source.settlement:n>source.history.length;
    const done=final?!!view.settlement:n<=view.history.length;
    return button(`<span class="step-node">${n===0?icon('box'):final?icon('coin'):label(n)}</span><span>${esc(text)}</span>`,'step:'+n,`${selected===n&&!disabled?'selected':''} ${done?'done':''}`,disabled?'disabled':'');
  }).join('')}</div><div class="round-actions">${button(icon('back'),'prev','round-arrow',`aria-label="${tr('previous')}" ${selected===0?'disabled':''}`)}${button(icon('arrow'),'next','round-arrow',`aria-label="${tr('next')}" ${selected>=maxFrame(source)?'disabled':''}`)}${button(tr(source.settlement?'toSettlement':'toLatest'),'latest','round-latest')}</div></div></section>`;
}
function bids(view) {
  return `<div class="table-scroll"><table class="bid-table"><thead><tr><th scope="col">${tr('agent')}</th>${Array.from({length:view.rules.rounds},(_,i)=>`<th scope="col">${label(i+1)}${i===view.settlement?.round-1?`<small>${tr('finalRound')}</small>`:''}</th>`).join('')}</tr></thead><tbody>${view.players.map((player,i)=>`<tr><th scope="row">${button(`<i class="agent-color-${i}"></i>${esc(player.name)}`,'agent:'+player.id,'text-button agent-name')}</th>${Array.from({length:view.rules.rounds},(_,r)=>{
    const record=view.history.find(h=>h.round===r+1),bid=record?.bids.find(b=>b.player_id===player.id);
    const high=bid && bid.amount===Math.max(...record.bids.map(b=>b.amount));
    return `<td class="${high?'high':''}">${bid?money(bid.amount):r+1===view.round && view.phase==='bidding'?`<span class="sealed-bid">${tr('sealed')}</span>`:'—'}</td>`;
  }).join('')}</tr>`).join('')}</tbody></table>${view.players.length?'':empty('noBids')}</div>`;
}
function chart(view) {
  const latest=view.history.at(-1), highest=latest?Math.max(...latest.bids.map(b=>b.amount)):null;
  const left=47,right=345,max=Math.max(1,...view.history.flatMap(r=>r.bids.map(b=>b.amount)));
  const x=round=>left+(round-1)*(right-left)/Math.max(1,view.rules.rounds-1),y=amount=>18+124*(1-amount/max);
  const plot=view.history.length?`<div class="chart-key">${view.players.map((p,i)=>`<span><i class="agent-color-${i}"></i>${esc(p.name)}</span>`).join('')}</div><svg class="chart refined-plot" role="img" aria-label="${tr('chartAria')}" viewBox="0 0 360 184">${[0,.5,1].map(r=>`<line x1="${left}" x2="${right}" y1="${18+124*r}" y2="${18+124*r}" stroke="#e5e7e2" stroke-dasharray="3 4"/><text x="8" y="${22+124*r}" fill="#777c78" font-size="11">${money(max*(1-r))}</text>`).join('')}${view.players.map((p,i)=>{
    const points=view.history.flatMap(r=>{const bid=r.bids.find(b=>b.player_id===p.id);return bid?[[x(r.round),y(bid.amount)]]:[];});
    return `<polyline fill="none" stroke="${colors[i]}" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round" points="${points.map(p=>p.join(',')).join(' ')}"/>${points.map(([px,py])=>`<circle cx="${px}" cy="${py}" r="3.5" fill="white" stroke="${colors[i]}" stroke-width="2"/>`).join('')}`;
  }).join('')}${Array.from({length:view.rules.rounds},(_,i)=>`<text x="${x(i+1)}" y="174" text-anchor="middle" fill="#777c78" font-size="11">${tr('round',{round:i+1})}</text>`).join('')}</svg>`:`<div class="chart-empty">${tr('noBids')}</div>`;
  return panel('trend',`<div class="signal-summary"><span>${tr('highest')}</span><strong>${highest===null?'—':money(highest)}<small>${tr('coins')}</small></strong></div>${plot}`,`<span class="unit">${latest?tr('round',{round:latest.round}):'—'}</span>`,'signal-panel');
}
function seats(view) {
  return `<div class="seats">${view.players.length?view.players.map((p,i)=>{
    const text=view.phase==='settled'?t(p.id===view.settlement.winner_id?'wonStatus':'lostStatus',{profit:signed(view.settlement.profit)}):
      t(view.phase==='lobby'?'seated':p.submitted?'submitted':'waitingBid');
    return button(`<span class="avatar agent-color-${i}">${label(i+1)}</span><span class="seat-name">${esc(p.name)}<small>${esc(text)}</small></span><span class="seat-dot ${p.submitted?'done':''}" aria-hidden="true"></span>`,'agent:'+p.id,'seat');
  }).join(''):`<div class="empty">${tr('startReady',{count:0,total:view.rules.player_count})}</div>`}</div>`;
}
function crate(view) {
  const w=view.rules.box_width,h=view.rules.box_height,items=knownItems(view);
  const drawing=`<div class="box-wrap"><div class="box-axis mono">(0, 0)<span>${tr('dimensions',{width:w,height:h})}</span></div><div class="box-stage"><svg class="crate" viewBox="0 0 ${w} ${h}" aria-label="${tr('boxAria',{width:w,height:h})}" role="img"><defs><pattern id="grid" width="1" height="1" patternUnits="userSpaceOnUse"><path d="M 1 0 L 0 0 0 1" fill="none" stroke="currentColor" stroke-width=".024"/></pattern></defs><rect width="${w}" height="${h}" fill="var(--box-bg)"/>${items.map(item=>`<g data-item="${esc(item.id)}" tabindex="0" role="button" aria-label="${esc(item.id)}"><rect x="${item.top_left.x}" y="${item.top_left.y}" width="${item.bottom_right.x-item.top_left.x}" height="${item.bottom_right.y-item.top_left.y}" fill="${itemColors[item.quality]||'#899cbd'}" fill-opacity="${view.settlement?'.7':'.45'}" stroke="var(--box-bg)" stroke-width=".08"/><title>${esc(item.id)} · ${quality(item.quality)}${item.price===undefined?'':' · '+money(item.price)+' '+tr('coins')}</title></g>`).join('')}<rect width="${w}" height="${h}" fill="url(#grid)" opacity=".4" pointer-events="none"/></svg></div><div class="box-caption"><span>${tr(view.settlement?'revealedCount':'knownCount',{count:items.length})}</span></div></div>`;
  return panel('box',`<div class="case-display">${drawing}</div>`,`<span class="box-state">${icon(view.settlement?'box':'lock')}${tr(view.settlement?'opened':view.phase==='lobby'?'notStarted':'sealed')}</span>`,'evidence-panel');
}
function hints(view) {
  return panel('hints',`<div class="intel-list">${view.public_hints.map(hint=>{
    const d=hint.data;let title,body;
    if(hint.kind==='random_shape'){
      title=tr('shape');body=`<p><b>${d.bottom_right.x-d.top_left.x} × ${d.bottom_right.y-d.top_left.y}</b> <span>${tr('cells')} · ${esc(d.item_id)}</span></p><small class="mono">(${d.top_left.x}, ${d.top_left.y}) → (${d.bottom_right.x}, ${d.bottom_right.y})</small>`;
    }else if(hint.kind==='quality_average'){
      title=tr('qualityAverage',{quality:t(d.quality)});body=`<p class="intel-value">${money(d.average_price)}<small>${tr('coins')}</small></p>`;
    }else if(hint.kind==='random_quality'){
      title=tr('randomQuality');body=`<p>${esc(d.item_id)} · ${quality(d.quality)}</p>`;
    }else if(hint.kind==='item_count'){
      title=tr('itemCount');body=`<p class="intel-value">${money(d.count)}</p>`;
    }else{title=esc(hint.kind);body=`<p>${esc(JSON.stringify(d))}</p>`;}
    return `<article><div class="intel-marker">${icon(hint.kind==='random_shape'?'shape':'coin')}</div><div class="intel-copy"><span class="eyebrow">${tr('round',{round:hint.round})}</span><h3>${title}</h3>${body}</div></article>`;
  }).join('')||empty('noHints')}</div>`,`<span class="unit">${view.public_hints.length}</span>`,'intel-panel');
}
function settlement(view) {
  if(!view.settlement)return `<div class="pending-settlement"><span>${tr('pending')}</span></div>`;
  const r=view.settlement,winner=view.players.find(p=>p.id===r.winner_id);
  return `<div class="settlement"><div><p class="eyebrow">${tr('finalOutcome',{round:r.round,method:t(r.method==='instant'?'instantSale':r.tied_players.length>1?'tieWin':'highestWins')})}</p><h3>${tr('winner',{name:winner?.name||r.winner_id})}</h3></div>${details([['paid',money(r.paid)],['totalValue',money(r.total_value)],['profit',`<span class="${r.profit>=0?'positive':'negative'}">${signed(r.profit)}</span>`]])}</div>`;
}
function inventory(view,ui) {
  if(!view.settlement)return '';
  const items=view.settlement.items.filter(item=>(ui.quality==='all'||item.quality===ui.quality)&&item.id.toLowerCase().includes(ui.search.toLowerCase()));
  return panel('inventory',`<div class="inventory-controls"><label>${tr('quality')} <select data-control="quality" aria-label="${tr('quality')}"><option value="all">${tr('allQuality')}</option>${['common','uncommon','rare','epic','legendary'].map(q=>`<option value="${q}" ${ui.quality===q?'selected':''}>${tr(q)}</option>`).join('')}</select></label><input data-control="search" aria-label="${tr('searchItem')}" placeholder="${tr('searchItem')}" value="${esc(ui.search)}"><small>${tr('itemCountRatio',{count:items.length,total:view.settlement.items.length})}</small></div><div class="inventory-table table-scroll"><table><thead><tr>${['id','quality','area','bounds','value'].map(key=>`<th>${tr(key)}</th>`).join('')}</tr></thead><tbody>${items.map(item=>`<tr><td>${button(esc(item.id),'item:'+item.id,'text-button')}</td><td><i class="quality-${esc(item.quality)}"></i>${quality(item.quality)}</td><td>${item.bottom_right.x-item.top_left.x} × ${item.bottom_right.y-item.top_left.y}</td><td class="mono">(${item.top_left.x}, ${item.top_left.y}) → (${item.bottom_right.x}, ${item.bottom_right.y})</td><td>${money(item.price)}</td></tr>`).join('')}</tbody></table>${items.length?'':empty('noItems')}</div>`);
}
function stats(results,ui) {
  const rows=results.agents,profit=results.profit;
  const rate=value=>value===null?'—':money(value,2)+'%';
  return `<div class="stats-toolbar"><span class="unit">${tr('allResults')}</span><label>${tr('sort')} <select data-control="sort" aria-label="${tr('statsSort')}">${[['profit','cumulativeProfit'],['rate','profitRate'],['average','averageProfit'],['max_profit','maxProfit'],['max_loss','maxLoss']].map(([value,key])=>`<option value="${value}" ${ui.sort===value?'selected':''}>${tr(key)}</option>`).join('')}</select></label></div><div class="metrics">${[['sampleCount',results.games],['cumulativeProfit',results.games?signed(profit):'—'],['profitRate',rate(results.rate)]].map(([key,value])=>`<article><p>${tr(key)}</p><strong>${value}</strong></article>`).join('')}</div>${panel('scorecard',rows.length?`<div class="table-scroll"><table class="stats-table"><thead><tr>${['agent','participation','profitRate','averageProfit','maxProfit','maxLoss','cumulativeProfit'].map(key=>`<th>${tr(key)}</th>`).join('')}</tr></thead><tbody>${rows.map(row=>`<tr><th scope="row">${button(esc(row.name),'stats-agent:'+row.id,'text-button')}</th><td>${row.games}</td><td>${rate(row.rate)}</td><td class="${row.average>=0?'positive':'negative'}">${signed(row.average,2)}</td><td>${signed(row.max_profit)}</td><td class="${row.max_loss<0?'negative':''}">${signed(row.max_loss)}</td><td class="${row.profit>=0?'positive':'negative'}">${signed(row.profit)}</td></tr>`).join('')}</tbody></table></div>`:empty('noResults'))}`;
}

export function roomDirectory(rooms,ui){
  return `<div class="page-heading"><h1>${tr('rooms')}</h1>${ui.manage?button(tr('createRoom'),'create-room','quiet'):''}</div><div class="room-directory">${rooms.map(room=>`<article class="panel room-tile"><span class="eyebrow">${tr(room.phase==='lobby'?'waiting':room.phase==='settled'?'settled':'bidding')}</span><h2>${esc(room.name)}</h2><div class="room-tile-footer"><span>${room.players} / ${room.player_count} ${tr('players')}</span>${button(tr('enterRoom'),'room:'+room.room_id,'quiet')}</div></article>`).join('')||`<div class="empty-room">${icon('brand')}<h2>${tr('noRooms')}</h2>${ui.manage?button(tr('createRoom'),'create-room','quiet'):''}</div>`}</div>`;
}
function decisionChoice(choice,view) {
  const definition=choice.action==='card'?view.rules.cards.find(card=>card.id===choice.card_id):null;
  const card=definition?localizeCard(definition):null;
  const tipId=`card-tip-${choice.round}-${choice.player_id}`;
  const skill=card?`<div class="decision-card"><strong>${esc(card.name)}</strong>${button('?',`card:${card.id}`,'card-help',`aria-label="${tr('cardHelp',{name:card.name})}" aria-describedby="${esc(tipId)}"`)}<div id="${esc(tipId)}" class="card-tooltip" role="tooltip">${esc(card.description)}</div></div>`:'';
  return `<div class="decision-choice"><span>${tr(choice.action==='card'?'cardDecision':'bidDecision')}</span>${skill}<p>${esc(choice.reason)}</p></div>`;
}
function decisions(view,ui) {
  if(!ui.observer)return panel('decisionReasons',`<div class="decision-access">${tr('openFromHost')}</div>`,'','decisions-panel');
  const selectedRound=ui.page==='replay' && ui.cursor!==null && ui.cursor<=view.history.length?ui.cursor:null;
  const rounds=selectedRound===null?Array.from({length:view.phase==='lobby'?0:view.round},(_,i)=>i+1):selectedRound?[selectedRound]:[];
  return panel('decisionReasons',`<div class="decision-rounds">${rounds.map(round=>`<section class="decision-round" data-key="reason-round:${round}"><h3>${tr('round',{round})}</h3><div class="decision-grid">${view.players.map(player=>{
    const choices=(view.decisions||[]).filter(d=>d.round===round&&d.player_id===player.id);
    return `<article data-key="reason-player:${round}:${esc(player.id)}"><h4>${esc(player.name)}</h4>${choices.map(choice=>decisionChoice(choice,view)).join('')||empty('noDecision')}</article>`;
  }).join('')}</div></section>`).join('')||empty(ui.page==='replay' && ui.cursor===0?'openingDecision':'noDecision')}</div>`,'','decisions-panel');
}
export function renderContent(source,results,ui) {
  if(ui.page==='stats')return `<div class="page-heading"><h1>${tr('agentStats')}</h1><span class="status-pill">${tr('gamesCount',{count:results.games})}</span></div>`+stats(results,ui);
  if(!source)return empty('loading');
  const view=observation(source,ui.cursor);
  const title=esc(source.room_name);
  const state=ui.page==='replay'?(ui.cursor===0?tr('opening'):view.settlement?tr('settled'):tr('round',{round:ui.cursor??view.round})):esc(status(view));
  const heading=`<div class="page-heading"><h1>${title}</h1><span class="status-pill"><i></i>${state}</span></div>`;
  const lobby=source.phase==='lobby'?`<div class="lobby-actions"><span>${tr('startReady',{count:source.players.length,total:source.rules.player_count})}</span>${ui.manage||source.can_manage?button(tr('invitePlayers'),'invite-room','quiet')+button(tr('start'),'start-room','quiet',source.players.length!==source.rules.player_count?'disabled':''):''}</div>`:'';
  if(source.phase==='lobby')return heading+lobby+panel('seats',seats(view),`<span class="unit">${view.players.length} / ${view.rules.player_count}</span>`,'seats-panel');
  return heading+lobby+`${timeline(source,view,ui.cursor)}${ui.page==='replay'?decisions(view,ui):''}<div class="ledger-grid"><div class="ledger-main">${panel('bids',bids(view),`<span class="unit">${tr('coins')}</span>`,'bids-panel')}${chart(view)}${panel('seats',seats(view),`<span class="unit">${view.players.length} / ${view.rules.player_count}</span>`,'seats-panel')}${settlement(view)}</div><aside class="ledger-aside">${crate(view)}${hints(view)}</aside></div>${ui.page==='replay'?'':decisions(view,ui)}${inventory(view,ui)}`;
}
export function dialogContent(kind,id,source,results,ui) {
  const view=source?observation(source,ui.cursor):null;
  if(kind==='create-room')return {title:t('createRoom'),content:`<form id="create-room-form" class="room-form"><label>${tr('roomName')}<input name="name" maxlength="32" required autocomplete="off"></label><label>${tr('players')}<select name="players">${[2,3,4,5,6,7].map(count=>`<option value="${count}" ${count===4?'selected':''}>${count}</option>`).join('')}</select></label><p id="create-room-error" class="negative" hidden>${tr('createRoomError')}</p><button type="submit" class="quiet">${tr('createRoom')}</button></form>`};
  if(kind==='invite-room')return {title:t('invitePlayers'),content:`<div class="invite-content"><label for="invite-link">${tr('inviteLink')}</label><div class="invite-copy"><input id="invite-link" readonly value="${esc(id)}">${button(tr('copyInvite'),'copy-invite','quiet')}</div><label>${tr('joinCommand')}</label><code class="join-command">bidbrawl join -invite "${esc(id)}"</code></div>`};
  if(kind==='room-error')return {title:t('roomActionError'),content:`<p>${tr('roomActionError')}</p>`};
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
