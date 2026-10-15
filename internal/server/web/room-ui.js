import {t,money,signed,localizeCard} from './i18n.js';
import {knownItems} from './model.js';
import {esc,icon,button} from './html.js';
import {avatar,avatars} from './avatars.js';
import {roomScene} from './room-scene.js';

const tr=(key,values)=>esc(t(key,values));
const selectedRound=(view,ui)=>ui.cursor===0?0:ui.cursor!==null&&ui.cursor<=view.history.length?ui.cursor:view.round;
const itemQuality=item=>item.quality?t(item.quality):t('unrevealed');

export function decisionChoice(choice,view){
  const definition=choice.action==='card'?(view.rules.cards||[]).find(card=>card.id===choice.card_id):null;
  const card=definition?localizeCard(definition):null,tip=`card-tip-${choice.round}-${choice.player_id}`;
  return `<article class="peek-thought"><span>${tr(choice.action==='card'?'cardDecision':'bidDecision')}</span>${card?`<div class="decision-card"><strong>${esc(card.name)}</strong>${button('?',`card:${card.id}`,'card-help',`aria-label="${tr('cardHelp',{name:card.name})}" aria-describedby="${esc(tip)}"`)}<div id="${esc(tip)}" class="card-tooltip" role="tooltip">${esc(card.description)}</div></div>`:''}<p>${esc(choice.reason)}</p></article>`;
}
export function playerPeek(source,view,ui){
  const p=view.players.find(p=>p.id===ui.focusPlayer);if(!p)return '';
  const round=selectedRound(view,ui),bid=view.history.find(h=>h.round===round)?.bids.find(b=>b.player_id===p.id);
  const last=source.frames.length-1,selected=ui.cursor??(source.settlement?last:-1);
  const rounds=source.phase==='lobby'?'':`<nav class="peek-rounds" aria-label="${tr('timeline')}">${[[t('opening'),0,false],...Array.from({length:source.rules.rounds},(_,i)=>[String(i+1).padStart(2,'0'),i+1,false]),[t('settlement'),last,true]].map(([label,n,final])=>button(esc(label),'step:'+n,selected===n&&(final?source.settlement:n<=source.history.length)?'selected':'',`aria-label="${n>0&&!final?tr('round',{round:n}):esc(label)}" aria-pressed="${selected===n&&(final?!!source.settlement:n<=source.history.length)}" ${(final?!source.settlement:n>source.history.length)?'disabled':''}`)).join('')}${source.phase==='bidding'?button(tr('toLatest'),'latest',ui.cursor===null?'selected':'',`aria-pressed="${ui.cursor===null}"`):''}</nav>`;
  const choices=ui.observer?(view.decisions||[]).filter(d=>d.player_id===p.id&&d.round===round):[];
  const side=view.players.findIndex(player=>player.id===p.id)%2?'right':'left';
  const payoff=view.settlement?.payoffs[p.id];
  return `<aside class="player-peek from-${side}" data-region="player-peek" role="dialog" aria-modal="false" aria-label="${tr('viewPlayer',{name:p.name})}" id="player-peek"><header>${avatar(p.avatar,{winner:view.settlement?.winner_id===p.id,context:'peek'})}<div><h2>${esc(p.name)}</h2></div>${button(icon('close'),'dismiss-player','icon-button',`aria-label="${tr('closePlayer')}"`)}</header>${rounds}<div class="peek-body"><div class="peek-bid"><span>${view.phase==='lobby'?tr('phaseLobby'):round===0?tr('opening'):tr('round',{round})}<small>${tr(bid?'bids':'bidNotRevealed')}</small></span><strong>${bid?money(bid.amount):'—'}<small>${tr('coins')}</small></strong></div>${payoff!==undefined?`<div class="peek-payoff"><span>${tr('peekReturn')}</span><strong class="${payoff>=0?'positive':'negative'}">${signed(payoff)}</strong></div>`:''}<div class="peek-thoughts" data-region="decisionReasons">${choices.map(d=>decisionChoice(d,view)).join('')||`<p class="peek-empty">${tr(view.phase==='lobby'?'notStarted':!ui.observer?'openFromHost':round===0?'openingDecision':'noDecision')}</p>`}</div><div class="peek-links">${button(tr('playerProfile'),'agent:'+p.id,'text-button')}${button(tr('allBids'),'bids','text-button')}</div></div>${ui.cursor!==null?`<footer>${button(tr('toLatest'),'latest','peek-latest')}</footer>`:''}</aside>`;
}
function seat(player,i,view,ui){
  const side=i%2?'right':'left',sidePlayers=view.players.filter((_,n)=>n%2===i%2),sideIndex=sidePlayers.findIndex(p=>p.id===player.id);
  const y=(sideIndex+1)/(sidePlayers.length+1)*100,winner=view.settlement?.winner_id===player.id;
  const label=winner?'crateWinner':view.phase==='lobby'?'seated':view.phase!=='settled'&&player.submitted?'submitted':null;
  return `<div class="world-seat seat-${side} ${winner?'seat-winner':''} ${ui.focusPlayer===player.id?'is-focused':''}" data-seat-y="${y}" data-key="seat:${esc(player.id)}">${button(`${avatar(player.avatar,{winner})}<strong>${esc(player.name)}</strong>${label?`<span class="seat-status">${tr(label)}</span>`:''}`,'focus-agent:'+player.id,'world-avatar-button',`aria-label="${tr('viewPlayer',{name:player.name})}" aria-expanded="${ui.focusPlayer===player.id}" aria-controls="player-peek"`)}</div>`;
}
export function crate(view,ui){
  const w=view.rules.box_width,h=view.rules.box_height,player=ui.observer?view.players.find(p=>p.id===ui.focusPlayer):null,items=knownItems(view,player?.id);
  const fog=view.phase!=='lobby'&&!view.settlement;
  const blocks=items.map((item,i)=>{
    const x=item.top_left.x,y=item.top_left.y,width=item.bottom_right.x-x,height=item.bottom_right.y-y;
    const label=`${item.id} · ${itemQuality(item)} · ${width}×${height}${item.price!==undefined?' · '+money(item.price)+' '+t('coins'):''}`;
    return button(`<span class="item-tile"><span class="item-number">${width>=2&&height>=2?String(i+1).padStart(2,'0'):''}</span></span>`,'select-item:'+item.id,`lot-item fit-item ${ui.focusItem===item.id?'is-selected':''}`,`data-item="${esc(item.id)}" data-quality="${esc(item.quality||'unrevealed')}" aria-pressed="${ui.focusItem===item.id}" aria-label="${esc(label)}" data-x="${x/w*100}" data-y="${y/h*100}" data-width="${width/w*100}" data-height="${height/h*100}"`);
  }).join('');
  return `<div class="crate-object"><div class="crate-lid"><span>${w} × ${h}</span><span>${tr(view.settlement?'revealedCount':player?'visibleCount':'knownCount',{count:items.length})}</span>${button(tr(ui.fitBox?'fillBoxView':'showWholeBox'),'fit-box','crate-fit-button',`aria-pressed="${!!ui.fitBox}" title="${tr('boxFitHelp')}"`)}</div><div class="crate-perspective" role="status">${icon(fog?'lock':'scan')}<span>${player?tr('playerVision',{name:player.name}):tr('publicVision')}</span><small>${tr(view.settlement?'fogRevealed':view.phase==='lobby'?'notStarted':'fogUnknown')}</small></div><div class="crate-face"><div class="crate-window ${ui.fitBox?'fit-all':'fill-view'}" tabindex="0" role="region" aria-label="${tr('boxFitHelp')}"><div class="crate-grid fitted-grid ${fog?'has-fog':''}" role="group" aria-label="${tr('boxAria',{width:w,height:h})}" data-columns="${w}" data-rows="${h}">${blocks}<span class="canvas-grid-lines" aria-hidden="true"></span></div>${!items.length?`<div class="crate-seal">${icon('lock')}<strong>${tr(view.phase==='lobby'?'notStarted':'fogCovered')}</strong></div>`:''}</div>${!ui.fitBox?`<span class="crate-pan-hint">${tr('boxPanHint')}</span>`:''}</div></div>`;
}
export function itemInfo(view,ui){
  const item=knownItems(view,ui.observer?ui.focusPlayer:null).find(item=>item.id===ui.focusItem);if(!item)return '';
  const w=item.bottom_right.x-item.top_left.x,h=item.bottom_right.y-item.top_left.y;
  return `<div class="inspector-main"><span class="inspector-kicker">${esc(itemQuality(item))}</span><h3>${esc(item.id.slice(0,8))}<small>${w} × ${h} ${tr('cells')}</small></h3></div><div class="inspector-price"><span>${tr('publicValue')}</span><strong>${item.price!==undefined?money(item.price):'—'}</strong><small>${tr(item.price!==undefined?'coins':'unrevealed')}</small></div><div class="inspector-actions">${button(tr('itemDetails'),'item:'+item.id,'item-detail-button')}${button(tr('unpin'),'unpin','unpin-button')}</div>`;
}
export function itemTip(item){
  const w=item.bottom_right.x-item.top_left.x,h=item.bottom_right.y-item.top_left.y;
  return `<span>${esc(itemQuality(item))}</span><strong>${esc(item.id.slice(0,8))}<em>${item.price!==undefined?money(item.price)+' '+tr('coins'):tr('unrevealed')}</em></strong><small>${w} × ${h} ${tr('cells')}</small>`;
}
export function avatarGallery(){return `<div class="avatar-gallery">${avatars.map((_,i)=>button(`${avatar(i,{large:true,context:'gallery'})}<strong>${tr('avatar'+i)}</strong>`,'avatar-greet:'+i,'avatar-specimen',`aria-label="${tr('sayHello')} · ${tr('avatar'+i)}"`)).join('')}</div>`;}
export function roomView(source,view,ui,state){
  const lobby=source.phase==='lobby'?`<div class="lobby-actions"><span>${tr('startReady',{count:source.players.length,total:source.rules.player_count})}</span>${ui.manage||source.can_manage?button(tr('invitePlayers'),'invite-room','quiet')+button(tr('start'),'start-room','quiet',source.players.length!==source.rules.player_count?'disabled':''):''}</div>`:'';
  return `<section class="auction-world" data-region="auction-world" aria-label="${tr('rooms')}"><header class="world-header"><div><h1>${esc(source.room_name)}</h1></div><span class="status-pill world-phase"><i></i>${state}</span><div class="world-tools">${button(`${icon('scan')}<span>${tr('hints')}</span><small>${view.public_hints.length}</small>`,'hints','world-intel-button')}${view.settlement?button(`${icon('box')}<span>${tr('inventory')}</span>`,'inventory','world-intel-button'):''}</div></header>${lobby}<div class="world-floor">${roomScene(source.room_id)}<div class="world-crate">${crate(view,ui)}</div><div class="world-seats" aria-label="${tr('seats')}">${view.players.map((p,i)=>seat(p,i,view,ui)).join('')}</div><div class="world-peek-region">${playerPeek(source,view,ui)}</div><aside class="lot-inspector" data-region="item-info" aria-label="${tr('itemDetails')}" ${knownItems(view,ui.observer?ui.focusPlayer:null).some(i=>i.id===ui.focusItem)?'':'hidden'}>${itemInfo(view,ui)}</aside></div></section>`;
}
