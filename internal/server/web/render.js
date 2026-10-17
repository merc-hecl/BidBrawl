import {t, money, signed, localizeCard, getLocale} from './i18n.js';
import {observation} from './model.js';
import {esc,icon,button} from './html.js';
import {roomView,avatarGallery} from './room-ui.js';
import {waterfall} from './illustrations.js';
import {roomActions,roomFilters,roomDate} from './room-management.js';
import {fullActivity} from './actions.js';
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
  return rooms.filter(room => !room.archived_at && roomGroup(room) === group).map(room => button(`<span class="room-dot ${esc(room.phase)}"></span><span>${esc(room.name)}<small>${tr('roomSeats',
    {count:room.players,total:room.player_count,phase:t({lobby:'phaseLobby',bidding:'phaseBidding',settled:'phaseSettled'}[room.phase])})}</small></span>`,
    'room:'+room.room_id, selected === room.room_id ? 'selected' : '', selected === room.room_id ? 'aria-current="true"' : '')).join('') || `<p class="side-empty">${tr(group === 'completed' ? 'noCompletedRooms' : 'noActiveRooms')}</p>`;
}
function bids(view) {
  const rounds=Array.from({length:view.rules.rounds},(_,i)=>{
    const record=view.history.find(h=>h.round===i+1),high=Math.max(0,...(record?.bids||[]).map(b=>b.amount));
    return {record,high,tied:high>0&&record.bids.filter(b=>b.amount===high).length>1,zero:!!record?.bids.length&&high===0};
  });
  return `<section class="bid-ledger"><p class="bid-ledger-subtitle">${icon('coin')}${tr('publicBidsNote')}</p><div class="bid-legend"><span class="bid-key">${tr('highestShort')}</span><span>${tr('roundHighest')}</span><span class="bid-key is-sale">${tr('settlement')}</span><span>${tr('settledBid')}</span></div><div class="table-scroll"><table class="bid-table"><thead><tr><th scope="col">${tr('players')}</th>${rounds.map(({zero},i)=>`<th scope="col">${label(i+1)}<small>${i===view.settlement?.round-1?tr('finalRound'):zero?tr('allZeroBids'):tr('round',{round:i+1})}</small></th>`).join('')}</tr></thead><tbody>${view.players.map(player=>`<tr><th scope="row">${button(esc(player.name),'agent:'+player.id,'text-button agent-name')}</th>${rounds.map(({record,high,tied},r)=>{
    const bid=record?.bids.find(b=>b.player_id===player.id),leader=!!bid&&high>0&&bid.amount===high;
    const sale=!!bid&&view.settlement?.round===r+1&&view.settlement.winner_id===player.id;
    return `<td><div class="bid-cell ${sale?'is-sale':leader?'is-highest':''}">${bid?`<strong>${bid.amount===0?tr('withdrawn'):money(bid.amount)}</strong>${sale?`<small>${icon('crown')}${tr('settledBid')}</small>`:leader?`<small>${tr(tied?'tiedHighest':'highestShort')}</small>`:''}`:r+1===view.round&&view.phase==='bidding'?`<span class="sealed-bid">${tr('sealed')}</span>`:'—'}</div></td>`;
  }).join('')}</tr>`).join('')}</tbody></table>${view.players.length?'':empty('noBids')}</div><p class="bid-ledger-note">${tr('bidLedgerNote')}</p></section>`;
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
 if(r.method==='unsold')return `<div class="settlement"><h3>${tr('unsold')}</h3>${details([['totalValue',money(r.total_value)],['profit',signed(r.profit)]])}<p>${tr('unsoldNote')}</p></div>`;
  return `<div class="settlement"><div><p class="eyebrow">${tr('finalOutcome',{round:r.round,method:t(r.method==='instant'?'instantSale':r.tied_players.length>1?'tieWin':'highestWins')})}</p><h3>${tr('winner',{name:winner?.name||r.winner_id})}</h3></div>${details([['paid',money(r.paid)],['totalValue',money(r.total_value)],['grossProfit',signed(r.gross_profit??r.profit)],['cardCost',money(r.total_card_cost||0)],['ticketFee',money(r.total_ticket_cost||0)],['easterEggValue',money(r.easter_egg_value||0)],['profit',`<span class="${r.profit>=0?'positive':'negative'}">${signed(r.profit)}</span>`]])}</div>`;
}
function inventory(view,ui) {
  if(!view.settlement)return '';
  const items=view.settlement.items.filter(item=>(ui.quality==='all'||item.quality===ui.quality)&&(item.id+' '+(item.name||'')+' '+(item.category||'')).toLowerCase().includes(ui.search.toLowerCase()));
  return `<section class="panel" data-region="inventory"><div class="inventory-controls"><label>${tr('quality')} <select data-control="quality" aria-label="${tr('quality')}"><option value="all">${tr('allQuality')}</option>${['common','uncommon','rare','epic','legendary'].map(q=>`<option value="${q}" ${ui.quality===q?'selected':''}>${tr(q)}</option>`).join('')}</select></label><input data-control="search" aria-label="${tr('searchItem')}" placeholder="${tr('searchItem')}" value="${esc(ui.search)}"><small>${tr('itemCountRatio',{count:items.length,total:view.settlement.items.length})}</small></div><div class="inventory-table table-scroll"><table><thead><tr>${['id','quality','area','bounds','value'].map(key=>`<th>${tr(key)}</th>`).join('')}</tr></thead><tbody>${items.map(item=>`<tr><td>${button(esc(item.name||item.id),'select-item:'+item.id,'text-button')}</td><td><i class="quality-${esc(item.quality)}"></i>${quality(item.quality)}</td><td>${item.bottom_right.x-item.top_left.x} × ${item.bottom_right.y-item.top_left.y}</td><td class="mono">(${item.top_left.x}, ${item.top_left.y}) → (${item.bottom_right.x}, ${item.bottom_right.y})</td><td>${money(item.price)}</td></tr>`).join('')}</tbody></table>${items.length?'':empty('noItems')}</div></section>`;
}
function stats(results,ui) {
  const rows=results.agents,profit=results.profit;
  const rate=value=>value===null?'—':money(value,2)+'%';
  const sortControl=`<label>${tr('sort')} <select data-control="sort" aria-label="${tr('statsSort')}">${[['profit','cumulativeProfit'],['rate','profitRate'],['average','averageProfit'],['max_profit','maxProfit'],['max_loss','maxLoss']].map(([value,key])=>`<option value="${value}" ${ui.sort===value?'selected':''}>${tr(key)}</option>`).join('')}</select></label>`;
  return `<div class="metrics">${[['sampleCount',results.games],['cumulativeProfit',results.games?signed(profit):'—'],['profitRate',rate(results.rate)],['totalPaid',money(results.paid)]].map(([key,value])=>`<article><p>${tr(key)}</p><strong class="${key==='cumulativeProfit'||key==='profitRate'?String(value).startsWith('−')||String(value).startsWith('-')?'negative':'positive':''}">${value}</strong></article>`).join('')}</div>${panel('scorecard',rows.length?`<div class="table-scroll"><table class="stats-table"><thead><tr>${['agent','participation','profitRate','averageProfit','maxProfit','maxLoss','cumulativeProfit'].map(key=>`<th>${tr(key)}</th>`).join('')}</tr></thead><tbody>${rows.map(row=>`<tr><th scope="row">${button(esc(row.name),'stats-agent:'+row.id,'text-button')}</th><td>${row.games}</td><td>${rate(row.rate)}</td><td class="${row.average>=0?'positive':'negative'}">${signed(row.average,2)}</td><td>${signed(row.max_profit)}</td><td class="${row.max_loss<0?'negative':''}">${signed(row.max_loss)}</td><td class="${row.profit>=0?'positive':'negative'}">${signed(row.profit)}</td></tr>`).join('')}</tbody></table></div>`:empty('noResults'),sortControl)}`;
}

const heading=(title,subtitle,action='')=>'<div class="page-heading"><div><h1>'+tr(title)+'</h1><p>'+tr(subtitle)+'</p></div>'+action+'</div>';
function roomTile(room,ui){
  return '<article class="paper room-tile '+(room.phase==='lobby'?'is-waiting':'')+'" data-room-card="'+esc(room.room_id)+'">'+roomActions(room,ui)+'<span class="eyebrow">'+esc(room.room_id.slice(0,8))+'</span><h2>'+esc(room.name)+'</h2><p>'+tr('roomSeats',{count:room.players,total:room.player_count,phase:t({lobby:'phaseLobby',bidding:'phaseBidding',settled:'phaseSettled'}[room.phase])})+'</p><div class="seat-marks" aria-hidden="true">'+Array.from({length:room.player_count},(_,i)=>'<span class="'+(i<room.players?'occupied':'')+'">'+icon('users')+'</span>').join('')+'</div>'+button(icon('arrow')+tr(room.phase==='settled'?'viewArchive':room.phase==='lobby'?'enterRoom':'enterWatch'),'room:'+room.room_id,'paper-button')+'</article>';
}
export function roomDirectory(rooms,ui){
  const ordered=rooms.filter(room=>!room.archived_at).sort((a,b)=>String(b.created_at||'').localeCompare(String(a.created_at||'')));
  const featured=ordered.find(room=>room.phase==='bidding')||ordered.find(room=>room.phase==='lobby')||ordered[0];
  const filtered=ui.directoryFilter==='waiting'?ordered.filter(room=>room.phase==='lobby'):ui.directoryFilter==='completed'?ordered.filter(room=>room.phase==='settled'):ordered;
  const create=ui.manage?button(icon('plus')+tr('createRoom'),'create-room','primary'):'';
  const hero='<section class="lobby-hero paper"><div class="hero-copy"><span class="eyebrow">'+tr('featured')+(featured?' · '+tr({lobby:'phaseLobby',bidding:'phaseBidding',settled:'phaseSettled'}[featured.phase]):'')+'</span><h2>'+esc(featured?.name||t('heroTitle'))+'</h2><p>'+tr('heroSubtitle')+'</p><div class="hero-note">'+icon('scan')+'<span>'+tr('heroTitle')+'</span></div>'+(featured?button(icon('arrow')+tr(featured.phase==='settled'?'viewArchive':'enterWatch'),'room:'+featured.room_id,'primary'):create)+'</div>'+waterfall()+'</section>';
  return heading('lobbyTitle','lobbySubtitle',create)+hero+'<div class="directory-heading"><h2>'+tr('findRoom')+'</h2>'+roomFilters(rooms,ui.directoryFilter||'all')+'</div><div class="room-directory">'+(filtered.map(room=>roomTile(room,ui)).join('')||'<div class="empty-room paper">'+icon('box')+'<h2>'+tr('noRooms')+'</h2>'+create+'</div>')+'</div>';
}
export function historyDirectory(rooms){
  const completed=rooms.filter(room=>room.phase==='settled').sort((a,b)=>String(b.created_at||'').localeCompare(String(a.created_at||'')));
  return heading('historyTitle','historySubtitle')+'<section class="history-list">'+(completed.map(room=>'<article class="paper history-card"><span class="archive-symbol">'+icon('book')+'</span><div><span class="eyebrow">'+esc(room.room_id.slice(0,8))+'</span><h2>'+esc(room.name)+'</h2><p>'+tr('roomSeats',{count:room.players,total:room.player_count,phase:t('phaseSettled')})+' · '+tr('createdAt')+' '+roomDate(room.created_at)+'</p></div>'+button(icon('arrow')+tr('viewArchive'),'room:'+room.room_id,'paper-button')+'</article>').join('')||'<div class="empty-room paper">'+icon('book')+'<h2>'+tr('noHistory')+'</h2></div>')+'</section>';
}
export function renderContent(source,results,ui) {
  if(ui.page==='stats')return heading('statsTitle','statsSubtitle')+stats(results,ui);
  if(!source)return empty('loading');
  const view=observation(source,ui.cursor);
  const state=ui.cursor!==null?(ui.cursor===0?tr('opening'):view.settlement?tr('settled'):tr('round',{round:ui.cursor})):esc(status(view));
  return roomView(source,view,ui,state);
}
export function dialogContent(kind,id,source,results,ui) {
  const view=source?observation(source,ui.cursor):null;
  if(kind==='room-access')return {title:t('createRoom'),content:`<p class="room-access-message">${tr('createRoomAccess')}</p>`};
  if(kind==='create-room')return {title:t('createRoom'),content:`<form id="create-room-form" class="room-form"><label>${tr('roomName')}<input name="name" maxlength="32" required autocomplete="off" placeholder="${tr('roomNamePlaceholder')}"></label><label>${tr('players')}<select name="players">${[2,3,4,5,6,7].map(count=>`<option value="${count}" ${count===4?'selected':''}>${count}</option>`).join('')}</select></label><label>${tr('warehouseType')}<select name="warehouse">${(ui.warehouses||[]).map(w=>`<option value="${esc(w.id)}">${esc(w.name)}</option>`).join('')}</select></label><label>${tr('ticketFee')}<input name="ticket" type="number" min="0" max="1000000000" step="1" value="5000" required></label><small>${tr('ticketNonrefundable')}</small><p id="create-room-error" class="negative" hidden>${tr('createRoomError')}</p><button type="submit" class="quiet">${tr('createRoom')}</button></form>`};
  if(kind==='invite-room')return {title:t('invitePlayers'),content:`<div class="invite-content"><label for="invite-link">${tr('inviteLink')}</label><div class="invite-copy"><input id="invite-link" readonly value="${esc(id)}">${button(tr('copyInvite'),'copy-invite','quiet')}</div><label>${tr('joinCommand')}</label><code class="join-command">bidbrawl join -invite "${esc(id)}"</code></div>`};
  if(kind==='room-error')return {title:t('roomActionError'),content:''};
  if(kind==='avatars')return {title:t('avatarGallery'),content:avatarGallery()};
  if(kind==='room-info')return view?{title:t('roomInfo'),content:'<section class="room-notes"><span class="eyebrow">'+esc(source.room_id)+'</span><h3>'+esc(source.room_name)+'</h3>'+details([['players',view.players.length+' / '+view.rules.player_count],['box',view.rules.box_width+' × '+view.rules.box_height],['rounds',String(view.rules.rounds)],['ticketFee',money(view.rules.ticket_fee||0)],['publicVision',tr(ui.observer?'observerWatch':'publicWatch')]])+'<div class="room-note-actions">'+button(icon('book')+tr('handbook'),'handbook','paper-button')+button(icon('help')+tr('rules'),'rules','paper-button')+button(icon('panel')+tr('roomSwitch'),'room-switch','paper-button')+(view.settlement?button(icon('box')+tr('viewContents'),'inventory','paper-button'):'')+button(icon('theme')+tr('paperTheme'),'toggle-theme','paper-button')+button(icon('language')+tr('language'),'toggle-language','paper-button')+button(icon('arrow')+tr('lobbyNav'),'back-lobby','paper-button')+'</div></section>'}:null;
  if(kind==='hints')return view?{title:t('hints'),content:hints(view)}:null;
  if(kind==='activities')return view&&ui.observer?{title:t('fullActivities'),content:fullActivity(view,ui)}:null;
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
    const pairs=[['players',tr('playersRule',{count:rules.player_count})],['rounds',tr('roundsRule',{count:rules.rounds})],['sale',(rules.early_settlement_multipliers||[]).some(Boolean)?tr('instantRule'):tr('saleRule',{round:rules.rounds})],['ties',tr('tieRule')],['publicClues',tr('publicCluesRule')],['bidPublication',tr('bidPublicationRule')],['privateSkills',tr('privateRule')],['box',tr('dimensions',{width:rules.box_width,height:rules.box_height})],['coordinates',tr('coordinatesRule')],['profit',tr('profitRule')],['otherReturn',tr('otherCostRule')]];
    const geometry=rules.game_config?.items;
    if(geometry)pairs.push(['itemMax',`${geometry.max_width} × ${geometry.max_height}`]);
    pairs.push(['ticketFee',money(rules.ticket_fee||0)+' · '+tr('ticketNonrefundable')]);
    pairs.push(['cardLimit',tr('cardsCount',{count:5})]);
    pairs.push(['warehouseType',esc((rules.warehouses||[]).find(w=>w.id===rules.warehouse)?.name||rules.warehouse||'mixed')]);
 pairs.push(['initialInspection',tr('inspectionRule',{count:rules.initial_inspection_count})]);
    return {title:t('rules'),content:button(icon('book')+tr('handbook'),'handbook','paper-button')+details(pairs).replace('<dl>','<dl class="rule-grid">')+`<details class="card-rules"><summary>${tr('cardCatalog')}</summary>${(rules.cards||[]).map(localizeCard).map(card=>`<article><h3>${esc(card.name)} · ${money(card.price||0)} ${tr('coins')}</h3><p>${esc(card.description)}</p></article>`).join('')}</details>`};
  }
  const seated=kind==='agent'?view?.players.find(player=>player.id===id):null;
  const record=ui.agentProfile || results.agents.find(agent=>agent.id===(kind==='stats-agent'?id:seated?.agent_id));
  const player=seated || record;if(!player)return null;
  const accountId=(player.agent_id||player.id).replace(/^account:/,'');
  const metrics=record?[['participation',money(record.games)],['profitRate',record.rate===null?'—':money(record.rate,2)+'%'],['averageProfit',signed(record.average,2)],['cumulativeProfit',signed(record.profit)],['maxProfit',signed(record.max_profit)],['maxLoss',signed(record.max_loss)],['cardCost',money(record.card_cost||0)],['ticketFee',money(record.ticket_cost||0)],['easterEggValue',money(record.easter_egg_value||0)],['regularProfit',signed(record.regular_profit??record.profit)]]:[];
  return {title:player.name,content:`<section class="agent-profile"><div class="agent-identity"><span>${tr('id')}</span><code>${esc(accountId)}</code></div><div class="agent-metrics">${metrics.map(([key,value])=>`<article class="${key==='profitRate'||key==='cumulativeProfit'?'featured':''}"><span>${tr(key)}</span><strong class="${/^[−-]/.test(value)?'negative':''}">${value}</strong></article>`).join('')}</div></section>`};
}
