import test from 'node:test';
import assert from 'node:assert/strict';
import {setLocale,t} from './web/i18n.js';
import {roomDirectory,roomButtons,historyDirectory} from './web/render.js';
import {archivedDirectory,deleteRoomsDialog} from './web/room-management.js';

const rooms=[
  {room_id:'waiting',name:'Occupied lobby',phase:'lobby',players:1,player_count:4},
  {room_id:'ongoing',name:'Ongoing',phase:'bidding',players:4,player_count:4},
  {room_id:'finished',name:'Finished',phase:'settled',players:4,player_count:4},
  {room_id:'archived',name:'<Archived>',phase:'settled',players:4,player_count:4,archived_at:'2026-10-16T01:00:00Z'}
];
test('main lobby manages every non-ongoing room and leaves bulk management to archives',()=>{
  setLocale('zh-CN',false);
  const html=roomDirectory(rooms,{manage:true,roomMenu:'waiting'});
  for(const id of ['waiting','finished'])assert.ok(html.includes('data-action="room-menu:'+id+'"'));
  assert.ok(!html.includes('data-action="room-menu:ongoing"'));
  assert.ok(html.includes('data-action="room-archive:waiting"')&&html.includes('data-action="room-delete:waiting"'));
  assert.ok(!html.includes('data-room-card="archived"')&&!html.includes('data-action="bulk-mode"'));
  assert.ok(!roomButtons(rooms,'','completed').includes('room:archived'));
  assert.ok(historyDirectory(rooms).includes('room:archived'));
  assert.ok(!roomDirectory(rooms,{manage:false}).includes('room-menu:'));
});
test('archive page provides secondary bulk entry, selection actions and read-only replay',()=>{
  for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    const ui={manage:true,bulkMode:false,selectedRooms:new Set()};
    const normal=archivedDirectory(rooms,ui);
    assert.ok(normal.includes('class="text-button batch-toggle"'));
    assert.ok(normal.includes('room-restore:archived')&&normal.includes('room-delete:archived'));
    assert.ok(!normal.includes('data-room-select=')&&!normal.includes('data-room-card="waiting"'));
    const bulk=archivedDirectory(rooms,{...ui,bulkMode:true,selectedRooms:new Set(['archived'])});
    assert.ok(bulk.includes('data-room-select="archived"'));
    assert.ok(bulk.includes('data-action="bulk-restore"')&&bulk.includes('data-action="bulk-delete"'));
    assert.ok(!bulk.includes('room-restore:archived'));
    const readonly=archivedDirectory(rooms,{...ui,manage:false});
    assert.ok(readonly.includes('room:archived')&&!readonly.includes('bulk-mode')&&!readonly.includes('room-delete:'));
  }
});
test('delete confirmation identifies all targets, escapes names and retains settled statistics',()=>{
  setLocale('zh-CN',false);
  const modal=deleteRoomsDialog([rooms[0],rooms[3]],{});
  assert.ok(modal.title.includes('2'));
  for(const text of [t('deleteStatsRetained'),t('deleteJoinedRoomNote'),'Occupied lobby','&lt;Archived&gt;'])assert.ok(modal.content.includes(text));
  assert.ok(!modal.content.includes('<Archived>'));
  assert.ok(modal.content.includes('confirm-room-delete')&&modal.content.includes('cancel-room-delete'));
});
