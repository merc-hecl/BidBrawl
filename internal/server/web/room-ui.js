import {t,money,signed,localizeCard,localizeName} from './i18n.js';
import {knownItems} from './model.js';
import {esc,icon,button} from './html.js';
import {avatar,avatars} from './avatars.js';
import {activityList,playerActivities,intelligenceSummary} from './actions.js';

const tr=(key,values)=>esc(t(key,values));
const selectedRound=(view,ui)=>ui.cursor===0?0:ui.cursor!==null&&ui.cursor<=view.history.length?ui.cursor:view.round;
const itemQuality=item=>item.category==='easter_egg'?t('easterEgg'):item.quality?t(item.quality):t('unrevealed');

export function decisionChoice(choice,view){
  const definition=choice.action==='card'?(view.rules.cards||[]).find(card=>card.id===choice.card_id):null;
  const card=definition?localizeCard(definition):null,tip=`card-tip-${choice.round}-${choice.player_id}`;
  return `<article class="peek-thought"><span>${tr(choice.action==='card'?'cardDecision':'bidDecision')}</span>${card?`<div class="decision-card"><strong>${esc(card.name)}</strong>${button('?',`card:${card.id}`,'card-help',`aria-label="${tr('cardHelp',{name:card.name})}" aria-describedby="${esc(tip)}"`)}<div id="${esc(tip)}" class="card-tooltip" role="tooltip">${esc(card.description)}</div></div>`:''}<p>${esc(choice.reason)}</p></article>`;
}
export function playerPeek(source,view,ui){
  return view.players.some(p=>p.id===ui.focusPlayer)?inspectionPanel(source,view,{...ui,inspector:'player'}):'';
}
function playerInfo(source,view,ui){
 const p=view.players.find(p=>p.id===ui.focusPlayer),round=selectedRound(view,ui),specialty=view.rules.specialties?.find(s=>s.id===p.specialty);
 const entries=ui.observer?playerActivities(view,p.id):[];
 const current=entries.filter(e=>e.action==='bid'&&e.status==='success'&&e.round===round).at(-1);
 const published=view.history.find(h=>h.round===round)?.bids.find(b=>b.player_id===p.id),bid=ui.observer&&current?{amount:current.params.amount}:published;
 const costs=view.player_costs?.[p.id],tab=ui.observer?(ui.activityTab||'actions'):'bids';
 const identity=`<header class="inspector-identity">${avatar(p.avatar,{winner:view.settlement?.winner_id===p.id,context:'peek'})}<div><h2>${esc(p.name)}</h2><p>${esc(localizeName('specialty',specialty))} · ${tr(ui.observer?'authorizedObserver':'publicVision')}${p.withdrawn?' · '+tr('withdrawn'):''}</p></div></header>`;
 const metrics=`<div class="player-action-metrics"><div><span>${tr(ui.observer?'currentOffer':'lastPublicOffer')}</span><strong>${bid?bid.amount===0?tr('withdrawn'):money(bid.amount):'—'}</strong></div>${ui.observer?`<div><span>${tr('cardCost')}</span><strong>${money(costs?.card_cost||0)}</strong></div><div><span>${tr('ticketFee')}</span><strong>${money(costs?.ticket_cost||0)}</strong></div>`:''}</div>`;
 const tabs=ui.observer?`<nav class="player-record-tabs">${['actions','intel','bids'].map(key=>button(tr('playerTab_'+key),'activity-tab:'+key,tab===key?'selected':'',`aria-pressed="${tab===key}"`)).join('')}</nav>`:'';
 let body;
 if(tab==='actions')body=activityList(view,{...ui,actionRound:ui.actionRound??round});
 else if(tab==='intel')body=`<div class="private-intel-records">${(view.intelligence?.[p.id]||[]).map(h=>`<article class="peek-thought"><span>${tr('round',{round:h.round})} · ${tr(h.source==='specialty'?'specialtyIntel':'privateSkills')}</span><strong>${esc(h.data.title||view.rules.cards?.find(c=>c.id===h.source)?.name||h.kind)}</strong><p>${esc(intelligenceSummary(h.data,view))}</p></article>`).join('')||`<p class="peek-empty">${tr('noHints')}</p>`}</div>`;
 else body=`<div class="player-bid-records">${view.history.map(h=>`<div><span>${tr('round',{round:h.round})}</span><strong>${h.bids.find(b=>b.player_id===p.id)?.amount===0?tr('withdrawn'):money(h.bids.find(b=>b.player_id===p.id)?.amount||0)}</strong></div>`).join('')}${view.phase==='bidding'?`<div><span>${tr('round',{round:view.round})}</span><small>${tr('bidNotRevealed')}</small></div>`:''}${!ui.observer?`<div class="activity-restricted">${icon('lock')}<strong>${tr('activityRestrictedTitle')}</strong><p>${tr('activityRestricted')}</p></div>`:''}</div>`;
 return `<div class="player-action-summary">${identity}${metrics}${tabs}</div><div class="player-tab-content" data-region="player-tab:${tab}">${body}</div>`;
}
function seat(player,i,view,ui){
  const winner=view.settlement?.winner_id===player.id;
  const bid=view.history.find(h=>h.round===selectedRound(view,ui))?.bids.find(b=>b.player_id===player.id);
  const label=winner?'crateWinner':player.withdrawn?'withdrawn':['lobby','opening'].includes(view.phase)?'seated':player.submitted?'submitted':'waitingBid';
  return '<div class="world-seat '+(winner?'seat-winner ':'')+(ui.focusPlayer===player.id?'is-focused':'')+'" data-region="seat:'+esc(player.id)+'">'+button(avatar(player.avatar,{winner})+'<strong>'+esc(player.name)+'</strong><span class="seat-status">'+(bid?bid.amount===0?tr('withdrawn'):money(bid.amount):tr(label))+'</span>','focus-agent:'+player.id,'world-avatar-button','aria-label="'+tr('viewPlayer',{name:player.name})+'" aria-expanded="'+(ui.focusPlayer===player.id)+'" aria-controls="room-inspector"')+'</div>';
}
export function crate(view,ui){
  const w=view.rules.box_width,h=view.rules.box_height,player=ui.observer?view.players.find(p=>p.id===ui.focusPlayer):null,items=knownItems(view,player?.id);
  const fog=!view.settlement&&(!view.items||!!player);
  const blocks=items.map((item,i)=>{
    const x=item.top_left.x,y=item.top_left.y,width=item.bottom_right.x-x,height=item.bottom_right.y-y;
    const label=item.id+' · '+itemQuality(item)+' · '+width+'×'+height+(item.price!==undefined?' · '+money(item.price)+' '+t('coins'):'');
    return button('<span class="item-meta"><span>'+esc(itemQuality(item))+'</span><span>'+width+' × '+height+'</span></span><span class="item-sketch" aria-hidden="true">'+icon('scan')+'<span>'+String(i+1).padStart(2,'0')+'</span></span><span class="item-caption"><strong>'+esc(item.name||item.id.slice(0,8))+'</strong><span>'+(item.price===undefined?'—':money(item.price))+'</span></span>','select-item:'+item.id,'lot-item '+(item.category==='easter_egg'?'is-easter-egg ':'')+(ui.focusItem===item.id?'is-selected':''),'data-item="'+esc(item.id)+'" data-quality="'+esc(item.quality||'unrevealed')+'" aria-pressed="'+(ui.focusItem===item.id)+'" aria-label="'+esc(label)+'" data-col="'+x+'" data-row="'+y+'" data-w="'+width+'" data-h="'+height+'"');
  }).join('');
  return '<div class="canvas-viewport" data-region="canvas:'+esc(view.room_id)+'" data-room="'+esc(view.room_id)+'" data-vision="'+esc(player?.id||(view.items?'observer':'public'))+'" tabindex="0" role="region" aria-label="'+tr('canvasHelp')+'"><div class="canvas-world '+(fog?'has-fog':'')+'" data-columns="'+w+'" data-rows="'+h+'" role="group" aria-label="'+tr('boxAria',{width:w,height:h})+'">'+blocks+'</div></div>'+(!items.length?'<div class="canvas-seal paper"><span class="eyebrow">'+icon('lock')+' LOT '+esc(view.room_id.slice(0,8))+'</span><h2>'+tr('sealedTitle')+'</h2></div>':'')+(player?'<div class="vision-label">'+tr('playerVision',{name:player.name})+'</div>':'');
}
function timeline(source,ui){
  const last=maxSourceFrame(source),selected=ui.cursor??(source.settlement?last:null);
  const rounds=Array.from({length:source.rules.rounds},(_,i)=>{
    const round=i+1,suffix=source.settlement&&round===last?' · '+tr('settlement'):'';
    return button(String(round).padStart(2,'0')+suffix,'step:'+round,selected===round?'selected':'','aria-label="'+tr('round',{round})+suffix+'" aria-pressed="'+(selected===round)+'" '+(round>source.history.length?'disabled':''));
  }).join('');
  return '<nav class="room-timeline" aria-label="'+tr('timeline')+'">'+button(tr('opening'),'step:0',selected===0?'selected':'','aria-pressed="'+(selected===0)+'"')+rounds+(source.settlement?'':button(tr('toLatest'),'latest',ui.cursor===null?'selected':'','aria-pressed="'+(ui.cursor===null)+'"'))+'</nav>';
}
const maxSourceFrame=source=>source.frames.length-1;
function waitingRoom(source,view,ui){
  const canManage=ui.manage||source.can_manage;
  return '<section class="waiting-room"><div class="page-heading"><div><span class="eyebrow">'+esc(source.room_id.slice(0,8))+'</span><h1>'+esc(source.room_name)+'</h1></div>'+button(icon('arrow')+tr('lobbyNav'),'back-lobby','paper-button')+'</div><section class="waiting-paper paper"><span class="eyebrow">'+tr('startReady',{count:source.players.length,total:source.rules.player_count})+'</span><h2>'+tr('waitingTitle')+'</h2><div class="waiting-seats">'+Array.from({length:source.rules.player_count},(_,i)=>view.players[i]?seat(view.players[i],i,view,ui):'<div class="empty-seat"><span class="empty-avatar">'+icon('users')+'</span><strong>'+tr('emptySeat')+'</strong><small>'+tr('seatNumber',{number:i+1})+'</small></div>').join('')+'</div><div class="waiting-actions">'+(canManage?button(icon('plus')+tr('invitePlayers'),'invite-room','paper-button')+button(icon('arrow')+tr('start'),'start-room','primary',source.players.length!==source.rules.player_count?'disabled':''):'<span>'+tr('phaseLobby')+'</span>')+'</div></section><div class="waiting-footer">'+button(icon('help')+tr('rules'),'rules','text-button')+button(icon('panel')+tr('roomSwitch'),'room-switch','text-button')+'</div>'+playerPeek(source,view,ui)+'</section>';
}
export function itemInfo(view,ui){
  const item=knownItems(view,ui.observer?ui.focusPlayer:null).find(item=>item.id===ui.focusItem);if(!item)return '';
  const w=item.bottom_right.x-item.top_left.x,h=item.bottom_right.y-item.top_left.y;
  const template=view.rules.catalog?.find(entry=>entry.id===item.template_id);
  const qualityTemplate=template||view.rules.catalog?.find(entry=>entry.quality===item.quality);
  const player=ui.observer?view.players.find(p=>p.id===ui.focusPlayer):null;
  // This diagram shows the observed footprint only; unknown quality and price stay unknown.
  const cell=Math.min(16,90/Math.max(w,h)),gap=Math.min(3,cell/4);
  const footprint=`<svg class="footprint-grid" width="${w*cell-gap}" height="${h*cell-gap}" viewBox="0 0 ${w*cell-gap} ${h*cell-gap}" aria-hidden="true">${Array.from({length:w*h},(_,i)=>`<rect x="${i%w*cell}" y="${Math.floor(i/w)*cell}" width="${cell-gap}" height="${cell-gap}" rx="${Math.min(3,cell/5)}"/>`).join('')}</svg>`;
  return `<header class="inspector-item-title ${item.category==='easter_egg'?'egg-item-title':''}"><span>${tr('itemRecord')}</span><h2>${esc(item.name||item.id.slice(0,8))}</h2><div><span>${esc(itemQuality(item))}</span></div></header><div class="inspector-item-metrics"><div class="inspector-price"><span>${tr('publicValue')}</span><div class="coin-amount">${item.price!==undefined?icon('coin'):''}<strong>${item.price!==undefined?money(item.price):'—'}</strong>${item.price!==undefined?`<span class="sr-only">${tr('coins')}</span>`:`<small>${tr('unrevealed')}</small>`}</div></div><div class="inspector-footprint">${footprint}<div><strong>${w} × ${h} ${tr('cells')}</strong><small>${tr('itemFootprint')}</small></div></div></div><dl class="inspector-facts"><div><dt>${tr('book.qualityOdds')}</dt><dd>${qualityTemplate?money(qualityTemplate.quality_appearance_probability*100,10)+'%':tr('unrevealed')}</dd></div><div><dt>${tr('book.appearanceOdds')}</dt><dd>${template?money(template.appearance_probability*100,10)+'%':tr('unrevealed')}</dd></div><div><dt>${tr('itemID')}</dt><dd class="inspector-id">${esc(item.id)}</dd></div><div><dt>${tr('itemPosition')}</dt><dd class="inspector-coordinates">[(${item.top_left.x}, ${item.top_left.y}), (${item.bottom_right.x}, ${item.bottom_right.y}))</dd></div><div><dt>${tr('itemPerspective')}</dt><dd>${player?tr('playerVision',{name:player.name}):view.items?tr('observerWatch'):tr('publicVision')}</dd></div></dl>`;
}
function inspectionPanel(source,view,ui){
  const player=view.players.find(p=>p.id===ui.focusPlayer),item=knownItems(view,ui.observer?ui.focusPlayer:null).find(i=>i.id===ui.focusItem);
  const kind=ui.inspector==='player'&&player?'player':ui.inspector==='item'&&item?'item':item?'item':player?'player':null;
  if(!kind)return '';
  const tabs=`<div class="inspector-tabs" role="tablist" aria-label="${tr('inspectionDetails')}">${['item','player'].map(tab=>button(tr(tab==='item'?'itemTab':'playerTab'),'inspect:'+tab,kind===tab?'selected':'',`role="tab" id="inspector-tab-${tab}" aria-selected="${kind===tab}" aria-controls="inspector-content" tabindex="${kind===tab?0:-1}" ${(tab==='item'?!item:!player)?'disabled':''}`)).join('')}</div>`;
  const actions=kind==='item'?button(tr('unpin'),'unpin','paper-button'):button(tr('playerProfile'),'agent:'+player.id,'paper-button')+button(tr(ui.observer?'fullActivities':'bidsShort'),ui.observer?'activities':'bids','paper-button');
  const showLatest=kind==='player'&&ui.cursor!==null&&(!source.settlement||ui.cursor<maxSourceFrame(source));
  return `<aside class="room-inspector paper ${kind==='player'?'player-peek':'lot-inspector'}" data-region="room-inspector" role="dialog" aria-modal="false" aria-label="${kind==='item'?tr('itemDetails'):tr('viewPlayer',{name:player.name})}" id="room-inspector"><header class="inspector-header">${tabs}${button(icon('close'),'dismiss-player','dialog-close',`aria-label="${tr('close')}"`)}</header><div class="inspector-body" data-region="inspect:${kind}:${kind==='item'?esc(item.id):esc(player.id)}" id="inspector-content" role="tabpanel" aria-labelledby="inspector-tab-${kind}" tabindex="0">${kind==='item'?itemInfo(view,ui):playerInfo(source,view,ui)}</div><footer class="inspector-footer">${actions}${showLatest?button(tr('toLatest'),'latest','peek-latest'):''}</footer></aside>`;
}
export function itemTip(item){
  const w=item.bottom_right.x-item.top_left.x,h=item.bottom_right.y-item.top_left.y;
  return `<span>${esc(itemQuality(item))}</span><strong>${esc(item.name||item.id.slice(0,8))}<em>${item.price!==undefined?money(item.price)+' '+tr('coins'):tr('unrevealed')}</em></strong><small>${w} × ${h} ${tr('cells')}</small>`;
}
export function avatarGallery(){return `<div class="avatar-gallery">${avatars.map((_,i)=>button(`${avatar(i,{large:true,context:'gallery'})}<strong>${tr('avatar'+i)}</strong>`,'avatar-greet:'+i,'avatar-specimen',`aria-label="${tr('sayHello')} · ${tr('avatar'+i)}"`)).join('')}</div>`;}
export function roomView(source,view,ui,state){
  if(source.phase==='lobby')return waitingRoom(source,view,ui);
  return '<section class="auction-world" data-region="auction-world" aria-label="'+tr('rooms')+'">'+crate(view,ui)+'<header class="room-heading paper">'+button(icon('arrow'),'back-lobby','back-button','aria-label="'+tr('lobbyNav')+'"')+'<div><span class="eyebrow room-brand"><img class="brand-avatar" src="/brand-avatar.png" width="18" height="18" alt="">BIDBRAWL / '+esc(source.room_id.slice(0,8))+'</span><h1>'+esc(source.room_name)+'</h1></div></header><div class="room-phase paper"><i class="status-dot"></i><span>'+state+'<small>'+roundTerms(view)+'</small></span></div><div class="room-menu">'+button(icon('users')+'<span>'+tr(ui.observer?'observerWatch':'publicWatch')+'</span>'+icon('more'),'room-info','paper-button','aria-label="'+tr('roomMenu')+'" aria-haspopup="dialog"')+'</div>'+((source.settlement||ui.cursor!==null||ui.focusPlayer)?timeline(source,ui):'')+'<div class="world-seats paper" aria-label="'+tr('seats')+'">'+view.players.map((p,i)=>seat(p,i,view,ui)).join('')+'</div><nav class="room-dock paper" aria-label="'+tr('roomInfo')+'">'+button(icon('book')+'<span>'+tr('cluesShort')+'</span><small>'+String(view.public_hints.length).padStart(2,'0')+'</small>','hints','dock-button')+button(icon('clock')+'<span>'+tr('bidsShort')+'</span><small>'+String(view.history.length).padStart(2,'0')+'</small>','bids','dock-button')+button(icon('users')+'<span>'+tr('roomInfo')+'</span>','room-info','dock-button')+'</nav><div class="canvas-tools paper">'+button('−','zoom-out','icon-button','aria-label="'+tr('zoomOut')+'"')+'<output class="zoom-value" aria-label="'+tr('zoom')+'">100%</output>'+button('+','zoom-in','icon-button','aria-label="'+tr('zoomIn')+'"')+button(icon('scan')+'<span>'+tr('showWholeBox')+'</span>','fit-box','fit-button','aria-label="'+tr('showWholeBox')+'"')+'</div>'+inspectionPanel(source,view,ui)+'</section>';
}

function intelMessage(h,view){const d=h.data;if(d.items)return t('intelItems');if(d.total_value!==undefined)return t('intelValue',{value:money(d.total_value)});if(d.count!==undefined)return t('intelCount',{count:d.count});if(d.area!==undefined)return t('intelArea',{count:d.area});if(d.counts)return Object.entries(d.counts).map(([id,n])=>`${(view.rules.categories||[]).find(c=>c.id===id)?.name||((view.rules.qualities||[]).find(q=>q.id===id)?.name)||id} ${n}`).join(' · ');return t('intelItems');}

function roundTerms(view){if(view.round===0)return '';if(view.settlement)return tr(view.settlement.method==='unsold'?'unsold':'inventoryNote');const ratio=view.rules.early_settlement_multipliers?.[view.round-1];return ratio?tr('roundThreshold',{ratio}):tr('finalPriceRule');}
