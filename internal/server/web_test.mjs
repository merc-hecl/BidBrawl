import test from 'node:test';
import assert from 'node:assert/strict';
import {normalizeView,observation,knownItems} from './web/model.js';
import {renderContent} from './web/render.js';
import {itemInfo} from './web/room-ui.js';

const emptyStats={games:0,profit:0,paid:0,rate:null,agents:[]};
// Fixture frames represent the server projection consumed by the renderer.
function withFrames(s){s={...s,activities:(s.decisions||[]).map((d,i)=>({...d,id:i+1,source:'server',status:'success',params:d.action==='card'?{card:d.card_id}:{amount:100},tool:'POST /api/'+d.action,at:'2026-10-17T08:00:00Z'}))};return {...s,frames:Array.from({length:s.history.length+1},(_,i)=>({...s,phase:s.settlement&&i>=s.history.length?'settled':i===0?'opening':'revealed',round:i,history:s.history.slice(0,i),activities:s.activities.filter(e=>e.round<=i),public_hints:s.public_hints.filter(h=>h.round<=i),decisions:s.decisions.filter(d=>d.round<=i),settlement:i>=s.history.length?s.settlement:undefined}))};}
const ui={room:'r',page:'room',cursor:2,sort:'profit',quality:'all',search:'',focusPlayer:'p1'};
let source={room_id:'r',room_name:'<Room>',phase:'settled',round:5,rules:{initial_inspection_count:3,rounds:5,box_width:20,box_height:30,player_count:2},players:[{id:'p1',name:'<Atlas>',avatar:5,submitted:true},{id:'p2',name:'Echo',avatar:1,submitted:true}],history:Array.from({length:5},(_,i)=>({round:i+1,bids:[{player_id:'p1',amount:100+i},{player_id:'p2',amount:200+i}]})),public_hints:[{round:1,kind:'random_shape',data:{title:'随机物品轮廓',message:'物品 known 占 1 × 1 格。',fact:{type:'random_shape',item_id:'known',top_left:{x:0,y:0},bottom_right:{x:1,y:1}},translations:{en:{title:'Random item shape',message:'Item known occupies 1 × 1 cells.'}}}},{round:3,kind:'quality_count',data:{title:'指定品质数量',message:'箱内稀有品质物品共有 2 件。',fact:{type:'quality_count',quality:'rare',count:2}}}],settlement:{winner_id:'p2',paid:204,total_value:300,profit:96,tied_players:['p2'],items:[{id:'known',quality:'rare',price:100,top_left:{x:0,y:0},bottom_right:{x:1,y:1}},{id:'hidden-item',quality:'common',price:200,top_left:{x:1,y:0},bottom_right:{x:2,y:1}}],round:5,method:'final',payoffs:{p1:0,p2:96}},decisions:[]};

source=withFrames(source);

test('replay omits future bids, hidden items and private fields; final reveal is atomic',()=>{
  const publicView=normalizeView({...source,self:{deck:['private'],bid:42}});
  assert.equal('self' in publicView,false);
  const fourth=observation(publicView,4);
  assert.equal(fourth.history.length,4);assert.equal(fourth.settlement,undefined);
  assert.equal(knownItems(fourth).length,1);
  const html=renderContent(publicView,emptyStats,{...ui,cursor:4});
  assert.ok(!html.includes('hidden-item'));
  const fifth=observation(publicView,5);
  assert.equal(fifth.history.length,5);assert.equal(fifth.settlement,publicView.settlement);
  assert.ok(html.includes('&lt;Atlas&gt;'));assert.ok(!html.includes('<Atlas>'));
});
test('instant sale replay and observer reasons do not expose later rounds or execute markup',()=>{
  const early=withFrames({...source,round:1,history:source.history.slice(0,1),public_hints:source.public_hints.slice(0,1),settlement:{...source.settlement,round:1,method:'instant'},decisions:[{round:1,player_id:'p1',action:'bid',reason:'<img src=x onerror=alert(1)>\nEstimate and risk'}]});
  assert.equal(observation(early,0).settlement,undefined);
  assert.equal(observation(early,0).decisions.length,0);
  assert.equal(observation(early,1).settlement.round,1);
  const html=renderContent(early,emptyStats,{...ui,cursor:null,observer:true});
  assert.ok(html.includes('&lt;img src=x onerror=alert(1)&gt;'));assert.ok(!html.includes('<img src=x'));
  assert.ok(!renderContent(early,emptyStats,{...ui,cursor:null,observer:false}).includes('Estimate and risk'));
});

test('observer sees the full box in all frames, selected players keep their vision and session loss restores public fog',()=>{
  const first={id:'private-first',quality:'rare',top_left:{x:0,y:0},bottom_right:{x:2,y:2}};
  const second={id:'private-second',quality:'epic',top_left:{x:2,y:0},bottom_right:{x:4,y:2}};
  const items=[{...first,name:'First',price:123},{...second,name:'Second',price:456}];
  const raw=withFrames({...source,phase:'bidding',settlement:undefined,items,total_value:579,initial_inspections:{p1:[first],p2:[second]},public_hints:[]});
  const observed=normalizeView(raw,true);
  for(const cursor of [null,0,1,4]){
    const view=observation(observed,cursor),state={...ui,cursor,observer:true,focusPlayer:null};
    assert.deepEqual(knownItems(view),items);
    const html=renderContent(observed,emptyStats,state);
    assert.ok(html.includes('data-item="private-first"')&&html.includes('data-item="private-second"'));
    const info=itemInfo(view,{...state,focusItem:'private-second'});
    assert.ok(info.includes('456'));
    for(const [focusPlayer,known,hidden] of [['p1',first,second],['p2',second,first]]){
      assert.deepEqual(knownItems(view,focusPlayer),[known]);
      const selected={...state,focusPlayer};
      const html=renderContent(observed,emptyStats,selected);
      assert.ok(html.includes(`data-item="${known.id}"`)&&!html.includes(`data-item="${hidden.id}"`));
      assert.ok(!itemInfo(view,{...selected,focusItem:known.id}).includes(String(items.find(item=>item.id===known.id).price)));
      assert.equal(itemInfo(view,{...selected,focusItem:hidden.id}),'');
    }
  }
  const expired=normalizeView(observed,false);
  assert.deepEqual(knownItems(observation(expired,0),'p1'),[]);
  const publicHTML=renderContent(expired,emptyStats,{...ui,cursor:0,observer:false});
  assert.ok(!publicHTML.includes('private-first')&&!publicHTML.includes('private-second'));
  assert.equal(expired.items,undefined);
  assert.equal(expired.total_value,undefined);
  assert.ok(expired.frames.every(frame=>frame.total_value===undefined));
  assert.ok(expired.frames.every(frame=>frame.items===undefined));
  const settled=renderContent(source,emptyStats,{...ui,cursor:null,observer:true});
  assert.ok(settled.includes('data-item="hidden-item"'));
});
