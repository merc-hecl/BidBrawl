import {t,getLocale} from './i18n.js';
import {esc,icon,button} from './html.js';

const tr=(key,values)=>esc(t(key,values));
export const roomDate=value=>value?esc(new Intl.DateTimeFormat(getLocale(),{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(value))):'';
const disabled=ui=>ui.roomBusy?'disabled':'';

export function roomActions(room,ui){
  if(!ui.manage||room.phase==='bidding')return '';
  const open=ui.roomMenu===room.room_id;
  return `<div class="room-card-actions">${button(icon('more'),'room-menu:'+room.room_id,'paper-button room-more',`aria-label="${tr('manageNamedRoom',{name:room.name})}" aria-expanded="${open}" ${disabled(ui)}`)}${open?`<section class="paper room-card-menu" aria-label="${tr('manageRoom')}"><span class="eyebrow">${tr('manageRoom')}</span>${button(icon('archive')+tr('archiveRoom'),'room-archive:'+room.room_id,'text-button',disabled(ui))}${button(icon('trash')+tr('deleteRoom'),'room-delete:'+room.room_id,'text-button negative',disabled(ui))}<small>${tr('roomMenuNote')}</small></section>`:''}</div>`;
}

export function roomFilters(rooms,filter='all'){
  const active=rooms.filter(room=>!room.archived_at),archived=rooms.length-active.length;
  return `<div class="segmented room-filters" role="group" aria-label="${tr('roomGroups')}">${[['all',tr('allRooms')+' '+active.length],['waiting',tr('waitingRooms')],['completed',tr('completedRooms')],['archived',icon('archive')+tr('archivedRooms')+' '+archived]].map(([value,label])=>button(label,'filter:'+value,'paper-button',`aria-pressed="${filter===value}"`)).join('')}</div>`;
}

export function archivedDirectory(rooms,ui){
  const archived=rooms.filter(room=>room.archived_at).sort((a,b)=>b.archived_at.localeCompare(a.archived_at));
  const selected=ui.selectedRooms||new Set();
  const select=(room,all=false)=>`<input type="checkbox" ${all?'data-room-select-all':`data-room-select="${esc(room.room_id)}"`} aria-label="${all?tr('selectArchivedRooms'):tr('selectNamedRoom',{name:room.name})}" ${(all?archived.length>0&&archived.every(room=>selected.has(room.room_id)):selected.has(room.room_id))?'checked':''} ${disabled(ui)}>`;
  return `<div class="page-heading archive-heading"><div><h1>${tr('archivedTitle')}</h1><p>${tr('archivedSubtitle')}</p></div>${roomFilters(rooms,'archived')}</div><div class="archive-toolbar">${ui.bulkMode?`<label class="select-all">${select(null,true)}<span>${tr('selectArchivedRooms')} (${archived.length})</span></label>`:'<span></span>'}${ui.manage&&archived.length?button(tr(ui.bulkMode?'exitBulk':'bulkManage'),'bulk-mode','text-button batch-toggle',disabled(ui)):''}</div><section class="archive-room-list">${archived.map(room=>`<article class="paper archived-room" data-room-card="${esc(room.room_id)}">${ui.bulkMode?select(room):`<span class="archive-room-symbol">${icon('archive')}</span>`}<div class="archive-room-copy"><h2>${esc(room.name)}</h2><p>${esc(room.room_id.slice(0,8))} · ${tr('roomSeats',{count:room.players,total:room.player_count,phase:t(room.phase==='settled'?'phaseSettled':'phaseLobby')})} · ${tr('archivedAt')} ${roomDate(room.archived_at)}</p></div>${ui.bulkMode?'':`<div class="archive-room-actions">${button(icon(room.phase==='settled'?'book':'arrow')+tr(room.phase==='settled'?'viewArchive':'enterRoom'),'room:'+room.room_id,'paper-button')}${ui.manage?button(icon('restore')+tr('restoreRoom'),'room-restore:'+room.room_id,'paper-button',disabled(ui))+button(icon('trash')+tr('deleteRoom'),'room-delete:'+room.room_id,'paper-button negative',disabled(ui)):''}</div>`}</article>`).join('')||`<div class="empty-room paper">${icon('archive')}<h2>${tr('noArchivedRooms')}</h2></div>`}</section><p class="archive-note">${tr('archiveRetention')}</p>${ui.manage&&ui.bulkMode?`<section class="paper room-selection-bar" aria-label="${tr('bulkManage')}"><div><strong>${tr('selectedRooms',{count:selected.size})}</strong><p>${tr('bulkArchiveNote')}</p></div><div>${button(icon('restore')+tr('restoreRoom'),'bulk-restore','paper-button',selected.size?disabled(ui):'disabled')}${button(icon('trash')+tr('deleteSelected'),'bulk-delete','paper-button danger-button',selected.size?disabled(ui):'disabled')}</div></section>`:''}`;
}

export function deleteRoomsDialog(targets,ui){
  return {title:t(targets.length===1?'deleteRoomTitle':'deleteRoomsTitle',{count:targets.length}),content:`<div class="room-deletion"><span class="delete-symbol">${icon('trash')}</span><p>${tr('deleteRoomWarning')}</p><ul class="delete-targets">${targets.map(room=>`<li><h3>${esc(room.name)}</h3><p>${esc(room.room_id.slice(0,8))} · ${tr(room.archived_at?'archivedRooms':room.phase==='settled'?'phaseSettled':'phaseLobby')} · ${tr('roomSeats',{count:room.players,total:room.player_count,phase:t(room.phase==='settled'?'phaseSettled':'phaseLobby')})}</p></li>`).join('')}</ul>${targets.some(room=>room.phase==='lobby'&&room.players)?`<p>${tr('deleteJoinedRoomNote')}</p>`:''}<p>${tr('deleteStatsRetained')}</p>${ui.roomError?`<p class="negative" role="alert">${esc(ui.roomError)}</p>`:''}<div class="delete-room-actions">${button(tr('keepRoom'),'cancel-room-delete','paper-button',disabled(ui))}${button(icon('trash')+tr(targets.length===1?'deleteRoom':'deleteRoomCount',{count:targets.length}),'confirm-room-delete','paper-button danger-button',disabled(ui))}</div></div>`};
}

export function roomNotice(notice,ui){
  if(!notice&&!ui.roomError)return '';
  if(ui.roomError)return `<div class="paper room-notice negative" role="alert">${esc(ui.roomError)}</div>`;
  return `<div class="paper room-notice" role="status">${icon('check')}<span>${tr(notice.key,{count:notice.ids.length})}</span>${notice.undo?button(tr('undo'),'undo-room-archive','text-button',disabled(ui)):''}</div>`;
}
