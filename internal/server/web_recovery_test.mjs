import test from 'node:test';
import assert from 'node:assert/strict';
import {roomDirectory,roomButtons,renderContent,dialogContent} from './web/render.js';
import {setLocale,t} from './web/i18n.js';

test('recovery failure remains visible without offering room mutations in both languages',()=>{
  try{for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    const failed={room_id:'failed',name:'<Unrecoverable>',phase:'unavailable',players:0,player_count:0};
    const healthy={room_id:'healthy',name:'Healthy',phase:'bidding',players:2,player_count:2};
    const ui={manage:true};
    const onlyFailed=roomDirectory([failed],ui);
    assert.ok(onlyFailed.includes(t('roomUnavailable'))&&onlyFailed.includes('disabled'));
    assert.ok(onlyFailed.includes('&lt;Unrecoverable&gt;')&&!onlyFailed.includes('<Unrecoverable>'));
    assert.ok(!onlyFailed.includes('room-delete:failed')&&!onlyFailed.includes('room-archive:failed'));
    const rooms=roomDirectory([failed,healthy],ui);
    assert.ok(rooms.includes('data-action="room:healthy"'));
    const sidebar=roomButtons([failed],null,'active');
    assert.ok(sidebar.includes('disabled')&&sidebar.includes(t('phaseUnavailable')));
  }}finally{setLocale('zh-CN',false);}
});

test('incomplete history is marked on statistics and agent profiles',()=>{
  try{for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    const agent={id:'account:a',name:'A',games:1,profit:20,paid:100,rate:20,average:20,max_profit:20,max_loss:0,unavailable_rooms:1};
    const results={games:1,profit:20,paid:100,rate:20,agents:[agent],unavailable_rooms:1};
    assert.ok(renderContent(null,results,{page:'stats',sort:'profit'}).includes(t('statsIncomplete',{count:1})));
    assert.ok(dialogContent('stats-agent',agent.id,null,results,{}).content.includes(t('statsIncomplete',{count:1})));
  }}finally{setLocale('zh-CN',false);}
});
