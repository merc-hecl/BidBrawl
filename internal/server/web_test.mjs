import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {catalogs,t,setLocale,localizeCard,localizeName,localizeItemName} from './web/i18n.js';
import {normalizeView,observation,knownItems} from './web/model.js';
import {esc,renderContent,roomDirectory,dialogContent,status} from './web/render.js';
import {itemInfo,crate} from './web/room-ui.js';
import {actionDetail,actionRows,activityTabs,intelligenceSummary} from './web/actions.js';
import {itemArtSource,itemArtwork} from './web/item-art.js';
import {createHash} from 'node:crypto';

test('lobby waterfall uses unique public catalog items and preserves its selection across redraws and languages',async()=>{
 const catalog=JSON.parse(await readFile(new URL('../game/items.json',import.meta.url),'utf8')).items;
 const selected=html=>[...html.matchAll(/data-template="([^"]+)"/g)].map(match=>match[1]);
 let original;
 for(const locale of ['zh-CN','en']){
  setLocale(locale,false);
  const html=roomDirectory([],{},catalog),ids=selected(html);
  assert.equal(new Set(ids).size,9);
  if(original)assert.deepEqual(ids,original);else original=ids;
  for(const id of ids){
   const item=catalog.find(item=>item.id===id);
   assert.ok(item&&item.category!=='easter_egg');
   assert.ok(html.includes(`/item-art/${id}.svg`));
   assert.ok(html.includes(esc(localizeItemName(item,catalog))));
  }
  assert.deepEqual(selected(roomDirectory([],{directoryFilter:'waiting'},catalog)),ids);
 }
 assert.doesNotMatch(roomDirectory([],{},[]),/collection-card/);
 const egg=catalog.find(item=>item.category==='easter_egg');
 assert.doesNotMatch(roomDirectory([],{},[egg,{...catalog[0],id:'custom-no-art'}]),/collection-card/);
 setLocale('zh-CN',false);
});

test('every built-in item and easter egg has its exact exported Figma artwork',async()=>{
 const catalog=JSON.parse(await readFile(new URL('../game/items.json',import.meta.url),'utf8')).items;
 const provenance=JSON.parse(await readFile(new URL('../../docs/item-art.figma.json',import.meta.url),'utf8'));
 assert.equal(catalog.length,568);
 assert.equal(catalog.filter(item=>item.category==='easter_egg').length,8);
 assert.equal(Object.keys(provenance.items).length,catalog.length);
 for(const item of catalog){
  assert.equal(itemArtSource(item,catalog),`/item-art/${item.id}.svg`);
  const svg=await readFile(new URL(`./web/item-art/${item.id}.svg`,import.meta.url),'utf8');
  assert.match(svg,/<svg\b/);
  assert.doesNotMatch(svg,/<(?:script|foreignObject)\b|\bon\w+=|(?:href|src)="https?:/i);
  assert.equal(createHash('sha256').update(svg).digest('hex'),provenance.items[item.id].sha256,item.id);
 }
});

test('artwork never identifies an item from shape, quality, category, name or price guesses',async()=>{
 const catalog=JSON.parse(await readFile(new URL('../game/items.json',import.meta.url),'utf8')).items;
 const template=catalog[0],observed={id:'observed-id',quality:template.quality,category:template.category,price:template.price,top_left:{x:0,y:0},bottom_right:{x:1,y:1}};
 for(const item of [observed,{...observed,name:template.name},{...observed,template_id:template.id},{...observed,name:template.name,template_id:'../'+template.id},{...template,id:'custom-template'},{...template,name:'Different host item'}])assert.equal(itemArtwork(item,catalog),'');
 assert.ok(itemArtwork({...observed,template_id:template.id,name:template.name},catalog));
});

test('crate illustrations follow public, observer and selected-player disclosure including easter eggs',async()=>{
 const catalog=JSON.parse(await readFile(new URL('../game/items.json',import.meta.url),'utf8')).items;
 for(const template of [catalog[0],catalog.find(item=>item.category==='easter_egg')]){
  const item={...template,id:'instance-id',template_id:template.id,top_left:{x:1,y:1},bottom_right:{x:2,y:2}};
  const raw={room_id:'art-room',phase:'bidding',round:1,players:[{id:'player'}],rules:{catalog,box_width:5,box_height:5},public_hints:[],history:[],items:[item],initial_inspections:{player:[item]},item_reveals:{}};
  const state={observer:false,focusPlayer:null,focusItem:null};
  assert.doesNotMatch(crate(normalizeView(raw,false),state),/\/item-art\//);
  assert.match(crate(normalizeView(raw,true),{...state,observer:true}),/\/item-art\//);
  assert.doesNotMatch(crate(normalizeView(raw,true),{...state,observer:true,focusPlayer:'player'}),/\/item-art\//);
  raw.item_reveals.player=[{item_id:item.id,name:template.name,template_id:template.id,top_left:item.top_left,bottom_right:item.bottom_right}];
  assert.match(crate(normalizeView(raw,true),{...state,observer:true,focusPlayer:'player'}),/\/item-art\//);
  raw.settlement={items:[item]};
  assert.match(crate(normalizeView(raw,false),state),/\/item-art\//);
 }
});

test('area averages preserve units, precision and an empty result in both languages',()=>{
 for(const locale of ['zh-CN','en']){
  setLocale(locale,false);
  assert.equal(intelligenceSummary({average_area:2.5},{rules:{}}),t('intelAverageArea',{value:'2.5'}));
  assert.equal(intelligenceSummary({average_area:2.5,sample_count:3},{rules:{}}),t('intelSampleAverageArea',{count:3,value:'2.5'}));
  assert.equal(intelligenceSummary({average_area:null},{rules:{}}),t('intelAverageArea',{value:t('noCandidates')}));
 }
 setLocale('zh-CN',false);
});

test('card call details show combined quality and exact-area scopes',async()=>{
 const cards=JSON.parse(await readFile(new URL('../cards/default.json',import.meta.url),'utf8')).cards;
 cards.push({id:'four_cell_count',name:'Count',effect:{type:'count',area:4}});
 const view={rules:{cards,categories:[]}};
 try{for(const locale of ['zh-CN','en']){
  setLocale(locale,false);
  const detail=id=>actionDetail({action:'card',status:'success',round:1,params:{card:id},result:{data:{}},source:'server'},view);
  assert.ok(detail('four_cell_count').includes(t('book.areaScope',{count:4})));
  assert.ok(detail('precious_count').includes(t('epic')+' + '+t('legendary')));
 }}finally{setLocale('zh-CN',false);}
});

test('card call details show the private chosen target safely',async()=>{
 const cards=JSON.parse(await readFile(new URL('../cards/default.json',import.meta.url),'utf8')).cards;
 cards.push({id:'area_count',name:'Count',target:'area',effect:{type:'count'}});
 const view={rules:{cards,categories:[{id:'art',name:'书画艺术'}]}};
 try{for(const locale of ['zh-CN','en']){
  setLocale(locale,false);
  const detail=(card,target)=>actionDetail({action:'card',status:'success',round:3,params:{card,target},result:{data:{count:0}},source:'server'},view);
  assert.ok(detail('region_census','southeast').includes(t('book.region.southeast')));
  assert.ok(detail('area_count','6').includes(t('book.areaScope',{count:6})));
  assert.ok(detail('target_quality_count','legendary').includes(t('legendary')));
  assert.ok(detail('focused_appraisal','observed-id').includes(t('chosenItem',{id:'observed-id'})));
 }}finally{setLocale('zh-CN',false);}
});

test('item inspector separates quality and item odds without identifying hidden templates',()=>{
 const template={id:'ordinary',quality:'common',quality_appearance_probability:0.45,appearance_probability:0.002};
 const item={id:'visible',quality:'common',template_id:'ordinary',top_left:{x:0,y:0},bottom_right:{x:1,y:1}};
 const view={rules:{catalog:[template]},settlement:{items:[item]}};
 for(const locale of ['zh-CN','en']){
  setLocale(locale,false);
  const html=itemInfo(view,{focusItem:'visible'});
  assert.ok(html.includes(t('book.qualityOdds'))&&html.includes('45%'));
  assert.ok(html.includes(t('book.appearanceOdds'))&&html.includes('0.2%'));
  const unknown=itemInfo({...view,settlement:{items:[{...item,template_id:undefined}]}},{focusItem:'visible'});
  assert.ok(unknown.includes('45%')&&!unknown.includes('0.2%'));
  assert.ok(unknown.includes(t('unrevealed')));
 }
 setLocale('zh-CN',false);
});

test('catalog query details show localized game terms and match counts',()=>{
 const view={rules:{qualities:[{id:'rare',name:'稀有'}]},round:2};
 for(const locale of ['zh-CN','en']){
  setLocale(locale,false);
  const html=actionDetail({action:'catalog',round:2,source:'server',status:'success',params:{name:'<Glass>',size:'2x1',quality:'稀有'},result:{templates:0}},view);
  assert.ok(html.includes('&lt;Glass&gt;')&&html.includes('2 × 1'));
  assert.ok(html.includes(t('rare'))&&html.includes(t('catalogMatches',{count:0})));
  assert.ok(!html.includes('/api/')&&!html.includes('<Glass>'));
 }
 setLocale('zh-CN',false);
});

test('activity calls remain individually inspectable, escaped and observer-only',()=>{
 const v={rules:{cards:[]},round:2,phase:'bidding',history:[],public_hints:[],activities:[
  {id:1,player_id:'p1',round:2,action:'state',source:'server',status:'success'},
  {id:2,player_id:'p1',round:2,action:'state',source:'server',status:'success'},
  {id:3,player_id:'p2',round:2,action:'bid',reason:'other-player-secret',source:'server',status:'success',params:{amount:123}},
  {id:4,player_id:'p1',round:2,action:'withdraw',reason:'<script>unsafe</script>',source:'server',status:'rejected',params:{},result:{message:'<img bad>'}}
 ]};
 const rows=actionRows(v,'p1',2);
 assert.ok(rows.includes('× 2')&&rows.includes('action-detail:1')&&rows.includes('action-detail:2'));
 assert.ok(!rows.includes('other-player-secret')&&!rows.includes('<script>'));
 assert.ok(rows.includes('&lt;script&gt;'));
 assert.ok(actionDetail(v.activities[3],v).includes(t('activity_rejected')));
 const tabs=activityTabs(v,{actionRound:2});
 assert.match(tabs,/data-action="action-round:3"[^>]*disabled/);
 assert.deepEqual(normalizeView(v,false).activities,[]);
 assert.equal(normalizeView(v,true).activities.length,4);
});

test('egg outline requires learned category and retains exact rectangular bounds',()=>{
 const item={id:'egg',quality:'legendary',price:17500000,top_left:{x:2,y:3},bottom_right:{x:3,y:4}};
 const v={room_id:'r',rules:{box_width:20,box_height:30},players:[],history:[],public_hints:[],settlement:{items:[item]}};
 const state={observer:false,focusItem:null};
 assert.ok(!crate(v,state).includes('is-easter-egg'));
 item.category='easter_egg';
 const html=crate(v,state);
 assert.ok(html.includes('is-easter-egg')&&html.includes('data-col="2" data-row="3" data-w="1" data-h="1"'));
});

test('action details present game facts in both languages without technical payloads',async()=>{
 const {cards}=JSON.parse(await readFile(new URL('../cards/default.json',import.meta.url),'utf8'));
 const {categories,warehouses}=JSON.parse(await readFile(new URL('../game/items.json',import.meta.url),'utf8'));
 const specialties=JSON.parse(await readFile(new URL('../game/specialties.json',import.meta.url),'utf8'));
 const view={rules:{cards,categories,warehouses,specialties},history:[]};
 const base={id:24,round:0,source:'system',status:'success',tool:'auction',at:'2026-10-17T08:37:25Z',params:{},duration_ms:42};
 try{for(const locale of ['zh-CN','en']){
  setLocale(locale,false);
  const ticket=actionDetail({...base,action:'ticket',cost:5000},view);
  assert.ok(ticket.includes(t('ticketPaid'))&&ticket.includes(t('opening')));
  assert.match(ticket,/5,000/);
  assert.ok(!ticket.includes('auction')||locale==='en');
  assert.ok(!ticket.includes('{}')&&!ticket.includes('<code>')&&!ticket.includes('42 ms'));
  const free=actionDetail({...base,action:'ticket',cost:0},view);
  assert.match(free,/class="action-detail-amount">0</);
  const card=actionDetail({...base,round:3,source:'server',action:'card',params:{card:'quality_counts'},cost:12000,result:{data:{counts:{rare:2,legendary:1}}}},view);
  for(const key of ['card.quality_counts.name','card.quality_counts.description','wholeCrate','qualityCounts','rare','legendary'])assert.ok(card.includes(t(key)),key);
  const bid={...base,round:3,source:'server',action:'bid',tool:'POST /api/bid',params:{round:3,amount:232000},reason:'Player-authored reason'};
  const pending=actionDetail(bid,view),revealed=actionDetail(bid,{...view,history:[{round:3}]});
  assert.ok(pending.includes(t('offerLocked'))&&!pending.includes(t('offerRevealed')));
  assert.ok(revealed.includes(t('offerRevealed'))&&!revealed.includes(t('offerLocked')));
  assert.ok(revealed.includes(t('sealedOffer'))&&revealed.includes('232,000')&&revealed.includes(bid.reason));
  assert.ok(!revealed.includes('/api/')&&!revealed.includes('POST')&&!revealed.includes('amount:'));
  const handbook=actionDetail({...base,source:'cli',action:'handbook',status:'reported',params:{topic:'play',lang:'en'}},view);
  assert.ok(handbook.includes(t('topic_play'))&&handbook.includes(t('languageEnglish'))&&handbook.includes(t('source_cli')));
  const loadout=actionDetail({...base,action:'loadout',params:{specialty:'research',cards:[]}},view);
  for(const key of ['specialtyStyle.research.name','noCarriedCards'])assert.ok(loadout.includes(t(key)),key);
  const purchase=actionDetail({...base,action:'buy',params:{card:'quality_counts',quantity:2}},view);
  assert.ok(purchase.includes(t('cardQuantity',{count:2}))&&purchase.includes(t('purchasedCard')));
  for(const [kind,list] of Object.entries({category:categories,specialty:specialties,warehouse:warehouses}))for(const definition of list){
   const key=kind==='specialty'&&definition.style?`specialtyStyle.${definition.style}.name`:`${kind}.${definition.id}.name`;
   assert.equal(catalogs['zh-CN'][key],definition.name);
   assert.equal(localizeName(kind,definition),t(key));
   assert.equal(localizeName(kind,{...definition,name:'Custom name'}),'Custom name');
  }
  const rejected=actionDetail({...base,action:'loadout',status:'rejected',params:{cards:'invalid',round:'invalid',debug:'raw-payload'},result:{message:'POST /api/loadout failed'}},view);
  assert.ok(rejected.includes(t('actionRejected'))&&!rejected.includes('raw-payload')&&!rejected.includes('/api/'));
  if(locale==='en')for(const html of [ticket,free,card,pending,revealed,handbook,loadout,purchase,rejected])assert.doesNotMatch(html,/[\u4e00-\u9fff]/);
 }}finally{setLocale('zh-CN',false);}
});

const emptyStats={games:0,profit:0,paid:0,rate:null,agents:[]};
// Fixture frames represent the server projection consumed by the renderer.
function withFrames(s){s={...s,activities:(s.decisions||[]).map((d,i)=>({...d,id:i+1,source:'server',status:'success',params:d.action==='card'?{card:d.card_id}:{amount:100},tool:'POST /api/'+d.action,at:'2026-10-17T08:00:00Z'}))};return {...s,frames:Array.from({length:s.history.length+1},(_,i)=>({...s,phase:s.settlement&&i>=s.history.length?'settled':i===0?'opening':'revealed',round:i,history:s.history.slice(0,i),activities:s.activities.filter(e=>e.round<=i),public_hints:s.public_hints.filter(h=>h.round<=i),decisions:s.decisions.filter(d=>d.round<=i),settlement:i>=s.history.length?s.settlement:undefined}))};}
const ui={room:'r',page:'room',cursor:2,sort:'profit',quality:'all',search:'',focusPlayer:'p1'};
let source={room_id:'r',room_name:'<Room>',phase:'settled',round:5,rules:{initial_inspection_count:3,rounds:5,box_width:20,box_height:30,player_count:2},players:[{id:'p1',name:'<Atlas>',avatar:5,submitted:true},{id:'p2',name:'Echo',avatar:1,submitted:true}],history:Array.from({length:5},(_,i)=>({round:i+1,bids:[{player_id:'p1',amount:100+i},{player_id:'p2',amount:200+i}]})),public_hints:[{round:1,kind:'random_shape',data:{title:'随机物品轮廓',message:'物品 known 占 1 × 1 格。',fact:{type:'random_shape',item_id:'known',top_left:{x:0,y:0},bottom_right:{x:1,y:1}},translations:{en:{title:'Random item shape',message:'Item known occupies 1 × 1 cells.'}}}},{round:3,kind:'quality_count',data:{title:'指定品质数量',message:'箱内稀有品质物品共有 2 件。',fact:{type:'quality_count',quality:'rare',count:2}}}],settlement:{winner_id:'p2',paid:204,total_value:300,profit:96,tied_players:['p2'],items:[{id:'known',quality:'rare',price:100,top_left:{x:0,y:0},bottom_right:{x:1,y:1}},{id:'hidden-item',quality:'common',price:200,top_left:{x:1,y:0},bottom_right:{x:2,y:1}}],round:5,method:'final',payoffs:{p1:0,p2:96}},decisions:[]};

source=withFrames(source);

test('timeout zero stays a numeric offer until an explicit withdrawal in both languages',()=>{
  try{for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    const raw={...source,phase:'bidding',round:2,round_deadline:'2026-10-26T12:05:00Z',settlement:undefined,frames:[],public_hints:[],
      players:source.players.map(p=>({...p,withdrawn:false,withdraw_round:undefined})),
      history:[{round:1,bids:[{player_id:'p1',amount:0},{player_id:'p2',amount:100}]}]};
    const state={...ui,cursor:null,focusPlayer:'p1',observer:false,activityTab:'bids'};
    const view=normalizeView(raw);
    assert.equal(view.round_deadline,raw.round_deadline);
    assert.ok(status(view).includes(t('bidDeadline',{time:new Date(raw.round_deadline).toLocaleTimeString(locale)})));
    const ledger=dialogContent('bids',null,view,emptyStats,state).content;
    assert.ok(ledger.includes('<strong>0</strong>')&&!ledger.includes(t('withdrawn')));
    const room=renderContent(view,emptyStats,state);
    assert.ok(room.includes('<strong>0</strong>')&&!room.includes(t('withdrawn')));
    const withdrawn={...view,round:3,players:view.players.map(p=>p.id==='p1'?{...p,withdrawn:true,withdraw_round:2}:p),
      history:[...view.history,{round:2,bids:[{player_id:'p1',amount:0},{player_id:'p2',amount:100}]}]};
    const updated=dialogContent('bids',null,withdrawn,emptyStats,state).content;
    assert.ok(updated.includes('<strong>0</strong>')&&updated.includes('<strong>'+t('withdrawn')+'</strong>'));
  }}finally{setLocale('zh-CN',false);}
});

test('room directory is empty until creation and management is restricted',()=>{
  for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    assert.ok(roomDirectory([],{manage:true}).includes('data-action="create-room"'));
    assert.ok(!roomDirectory([],{manage:false}).includes('data-action="create-room"'));
    const rooms=roomDirectory([{room_id:'r',name:'<Table>',phase:'lobby',players:1,player_count:3}],{manage:true});
    assert.ok(rooms.includes('&lt;Table&gt;')&&rooms.includes('data-action="room:r"'));
    assert.ok(dialogContent('create-room',null,null,emptyStats,ui).content.includes('name="players"'));
    const access=dialogContent('room-access',null,null,emptyStats,{...ui,manage:false});
    assert.ok(access.content.includes(t('createRoomAccess'))&&!access.content.includes('<form'));
    assert.ok(dialogContent('invite-room','http://localhost/invite?room=r',null,emptyStats,ui).content.includes('bidbrawl join -invite'));
  }
});

test('statistics and agent profile work without a selected room and never filter by it',()=>{
  const results={games:1,profit:-15,paid:100,rate:-15,agents:[{id:'account:'+'a'.repeat(32),name:'Moss',games:1,profit:-15,paid:100,rate:-15,average:-15,max_profit:0,max_loss:-15}]};
  const html=renderContent(null,results,{...ui,page:'stats',room:'unrelated'});
  assert.ok(html.includes('Moss')&&!html.includes('data-control="scope"'));
  const profile=dialogContent('stats-agent','account:'+ 'a'.repeat(32),null,results,ui);
  assert.equal(profile.title,'Moss');

});
source.rules.cards=JSON.parse(await readFile(new URL('../cards/default.json',import.meta.url),'utf8')).cards;

test('approved public facts merge quality while ignoring removed price facts',()=>{
  const view={...source,settlement:undefined,public_hints:[
    {round:1,data:{fact:{type:'random_shape',item_id:'known',top_left:{x:0,y:0},bottom_right:{x:1,y:1}}}},
    {round:3,data:{fact:{type:'random_quality',item_id:'known',quality:'rare'}}},
    {round:3,data:{fact:{type:'quality_count',quality:'common',count:5}}},
    {round:3,data:{fact:{type:'random_quality_price',item_id:'unlocated',quality:'epic',price:999}}},
  ]};
  assert.deepEqual(knownItems(view),[{id:'known',top_left:{x:0,y:0},bottom_right:{x:1,y:1},quality:'rare'}]);
});

test('custom clue text is escaped, localized and cannot reveal item geometry',()=>{
  const data={title:'<script>title</script>',message:'<img src=x onerror=alert(1)>',translations:{en:{title:'Custom clue',message:'Perhaps a surprise.'}},item_id:'injected-item',top_left:{x:0,y:0},bottom_right:{x:1,y:1}};
  const view={...source,settlement:undefined,public_hints:[{round:1,kind:'unknown_custom_plugin',data}]};
  setLocale('zh-CN',false);
  const zh=dialogContent('hints',null,view,emptyStats,{...ui,cursor:null}).content;
  assert.ok(zh.includes('&lt;script&gt;')&&zh.includes('&lt;img'));
  assert.ok(!zh.includes('<script>')&&!zh.includes('<img'));
  assert.equal(knownItems(view).length,0);
  setLocale('en',false);
  const en=dialogContent('hints',null,view,emptyStats,{...ui,cursor:null}).content;
  assert.ok(en.includes('Custom clue')&&en.includes('Perhaps a surprise.'));
  assert.ok(!en.includes('total item count'));
  setLocale('zh-CN',false);
});

test('quality presence and largest area display without inventing item outlines',()=>{
  const facts=[{type:'quality_presence',quality:'legendary',present:false},{type:'quality_largest_area',quality:'rare',area:12}];
  const view={...source,settlement:undefined,public_hints:facts.map(fact=>({round:1,kind:fact.type,data:{title:'品质线索',message:'仅包含已公开事实',fact,translations:{en:{title:'Quality clue',message:'Only published facts'}}}}))};
  assert.deepEqual(knownItems(view),[]);
  for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    const html=dialogContent('hints',null,view,emptyStats,{...ui,cursor:null}).content;
    assert.equal((html.match(/<article>/g)||[]).length,2);
    assert.ok(html.includes(locale==='en'?'Only published facts':'仅包含已公开事实'));
  }
  setLocale('zh-CN',false);
});

test('locales have matching keys, placeholders and preserve custom card text',()=>{
  assert.deepEqual(Object.keys(catalogs.en).sort(),Object.keys(catalogs['zh-CN']).sort());
  for(const key of Object.keys(catalogs.en)) {
    const fields=text=>[...new Set((typeof text==='string'?[text]:Object.values(text)).flatMap(form=>[...form.matchAll(/\{(\w+)\}/g)].map(match=>match[1])))].sort();
    assert.deepEqual(fields(catalogs.en[key]),fields(catalogs['zh-CN'][key]),key);
  }
  for(const locale of ['zh-CN','en']) {
    setLocale(locale,false);
    for(const page of ['room','stats'])assert.doesNotThrow(()=>renderContent(source,emptyStats,{...ui,page}));
    assert.ok(dialogContent('rules',null,source,emptyStats,ui).content);
  }
  setLocale('zh-CN',false);assert.equal(t('round',{round:3}),'第 3 轮');
  setLocale('en',false);assert.equal(t('round',{round:3}),'Round 3');
  assert.equal(t('gamesCount',{count:1}),'1 game');assert.equal(t('gamesCount',{count:2}),'2 games');
  assert.throws(()=>t('missing-key'));
  const builtin=source.rules.cards;
  for(const card of builtin)assert.notEqual(localizeCard(card).description,card.description);
  const custom={...builtin[0],name:'My custom card'};assert.equal(localizeCard(custom),custom);
});
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
test('statistics render backend metrics without recalculating them',()=>{
 const summary={games:3,profit:60,paid:400,rate:15,agents:[{id:'account:a',name:'A',games:3,profit:10,paid:400,rate:2.5,average:10/3,max_profit:20,max_loss:-10}]};
 const html=renderContent(source,summary,{...ui,page:'stats'});
 assert.ok(html.includes('2.5%')&&html.includes('+3.33')&&html.includes('15%'));
 assert.ok(!html.includes('Win rate')&&!html.includes('胜率'));
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

test('settling round has a single timeline step and selection in both languages',()=>{
  for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    for(const round of [1,3,5]){
      const settled=withFrames({...source,round,history:source.history.slice(0,round),settlement:{...source.settlement,round}});
      for(const cursor of [null,round]){
        const html=renderContent(settled,emptyStats,{...ui,cursor});
        const timeline=html.match(/<nav class="room-timeline"[^>]*>(.*?)<\/nav>/s)[1];
        assert.equal((timeline.match(new RegExp(`data-action="step:${round}"`,'g'))||[]).length,1);
        assert.equal((timeline.match(/aria-pressed="true"/g)||[]).length,1);
        assert.ok(timeline.includes(`aria-label="${t('round',{round})} · ${t('settlement')}"`));
      }
    }
  }
});

test('opening and revealed round labels use matching terms; live selects only the current entry',()=>{
 for(const locale of ['zh-CN','en']){
  setLocale(locale,false);
  const aligned=withFrames({...source,rules:{...source.rules,early_settlement_multipliers:[2,1.6,1.3,1.1,0]}});
  for(let cursor=0;cursor<5;cursor++){
   const html=renderContent(aligned,emptyStats,{...ui,cursor});
   const phase=html.match(/<div class="room-phase paper">(.*?)<\/div>/s)[1];
   assert.ok(phase.includes(cursor===0?t('opening'):t('round',{round:cursor})));
  }
  const live=withFrames({...aligned,phase:'bidding',round:2,history:aligned.history.slice(0,1),settlement:undefined});
  const html=renderContent(live,emptyStats,{...ui,cursor:null});
  const timeline=html.match(/<nav class="room-timeline"[^>]*>(.*?)<\/nav>/s)[1];
  assert.equal((timeline.match(/aria-pressed="true"/g)||[]).length,1);
  assert.match(timeline,/data-action="latest"[^>]*aria-pressed="true"/);
 }
});

test('player pane shows only selected player and round reasons and marks restricted access',()=>{
  const recorded=withFrames({...source,decisions:[
    {round:1,player_id:'p1',action:'bid',reason:'first-round-rationale'},
    {round:2,player_id:'p2',action:'card',card_id:'item_count',reason:'second-round-rationale'}
  ]});
  const first=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:1,observer:true});
  assert.ok(first.includes('first-round-rationale'));
  assert.ok(!first.includes('second-round-rationale'));
  assert.ok(first.includes('data-action="activities"'));
  const second=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:2,observer:true,focusPlayer:'p2'});
  assert.ok(second.includes('second-round-rationale'));
  assert.ok(!second.includes('first-round-rationale'));
  const final=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:5,observer:true});
  assert.ok(!final.includes('first-round-rationale')&&!final.includes('second-round-rationale'));
  const opening=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:0,observer:true});
  assert.ok(opening.includes(t('noActivities'))&&!opening.includes('first-round-rationale'));
  const latest=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:null,observer:true});
  assert.ok(!latest.includes('first-round-rationale')&&!latest.includes('second-round-rationale'));
  const anonymous=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:1,observer:false});
  assert.ok(anonymous.includes(t('activityRestrictedTitle'))&&!anonymous.includes('data-action="observer-login"'));
  assert.ok(!anonymous.includes('first-round-rationale')&&!anonymous.includes('second-round-rationale'));
});

test('observer skill names and help use the saved card definition safely in both languages',()=>{
  const played=withFrames({...source,decisions:[{round:1,player_id:'p1',action:'card',card_id:'largest_quality',reason:'estimate'}]});
  for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    const html=renderContent(played,emptyStats,{...ui,page:'room',cursor:1,observer:true});
    assert.ok(html.includes(t('card.largest_quality.name')));
    const call=actionDetail(played.activities[0],played);
    assert.ok(call.includes(t('card.largest_quality.description'))&&call.includes('data-action="card:largest_quality"'));
    const detail=dialogContent('card','largest_quality',played,emptyStats,{...ui,observer:true});
    assert.equal(detail.title,t('card.largest_quality.name'));
    assert.ok(detail.content.includes(t('card.largest_quality.description')));
    assert.equal(dialogContent('card','largest_quality',played,emptyStats,{...ui,observer:false}),null);
  }
  const custom=withFrames({...played,rules:{...source.rules,cards:[{id:'custom',name:'<Card>',description:'<img src=x onerror=alert(1)>',effect:{type:'item_count'}}]},decisions:[{round:1,player_id:'p1',action:'card',card_id:'custom',reason:'estimate'}]});
  const html=renderContent(custom,emptyStats,{...ui,page:'room',cursor:1,observer:true});
  assert.ok(html.includes('&lt;Card&gt;'));
  assert.ok(actionDetail(custom.activities[0],custom).includes('&lt;img src=x onerror=alert(1)&gt;'));
  assert.ok(!html.includes('<img src=x'));
  const anonymous=renderContent(custom,emptyStats,{...ui,page:'room',cursor:1,observer:false});
  assert.ok(!anonymous.includes('Card')&&!anonymous.includes('onerror'));
});


test('room renders persisted avatars and item bounds',()=>{
  const html=renderContent(source,emptyStats,{...ui,cursor:null});
  assert.ok(html.includes('data-bot="5"')&&html.includes('data-bot="1"'));
  assert.ok(html.includes('data-col="0" data-row="0" data-w="1" data-h="1"'));
  assert.ok(html.includes('data-columns="20" data-rows="30"'));
  assert.equal((html.match(/data-item=/g)||[]).length,2);
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
    assert.ok(html.includes('class="crate-value"')&&html.includes('<strong>579</strong>'));
    assert.ok(html.includes('data-item="private-first"')&&html.includes('data-item="private-second"'));
    assert.ok(!html.includes('has-fog')&&!html.includes('canvas-seal')&&!html.includes('vision-label'));
    const info=itemInfo(view,{...state,focusItem:'private-second'});
    assert.ok(info.includes('456')&&info.includes(t('observerWatch')));
    for(const [focusPlayer,known,hidden] of [['p1',first,second],['p2',second,first]]){
      assert.deepEqual(knownItems(view,focusPlayer),[known]);
      const selected={...state,focusPlayer};
      const html=renderContent(observed,emptyStats,selected);
      assert.ok(!html.includes('class="crate-value"'));
      assert.ok(html.includes(`data-item="${known.id}"`)&&!html.includes(`data-item="${hidden.id}"`));
      assert.ok(html.includes('has-fog')&&html.includes('vision-label'));
      assert.ok(!itemInfo(view,{...selected,focusItem:known.id}).includes(String(items.find(item=>item.id===known.id).price)));
      assert.equal(itemInfo(view,{...selected,focusItem:hidden.id}),'');
    }
  }
  const expired=normalizeView(observed,false);
  assert.deepEqual(knownItems(observation(expired,0),'p1'),[]);
  const publicHTML=renderContent(expired,emptyStats,{...ui,cursor:0,observer:false});
  assert.ok(!publicHTML.includes('private-first')&&!publicHTML.includes('private-second'));
  assert.ok(publicHTML.includes('has-fog')&&publicHTML.includes('canvas-seal'));
  assert.equal(expired.items,undefined);
  assert.equal(expired.total_value,undefined);
  assert.ok(!publicHTML.includes('class="crate-value"'));
  assert.ok(expired.frames.every(frame=>frame.total_value===undefined));
  assert.ok(expired.frames.every(frame=>frame.items===undefined));
  const settled=renderContent(source,emptyStats,{...ui,cursor:null,observer:true});
  assert.ok(!settled.includes('has-fog')&&settled.includes('data-item="hidden-item"'));
});

test('skill reveals accumulate only learned attributes in the selected player vision',()=>{
  const inspected={id:'sample',quality:'rare',top_left:{x:0,y:0},bottom_right:{x:1,y:1}};
  const shape={round:1,item_id:'scanned',top_left:{x:1,y:0},bottom_right:{x:3,y:2}};
  const price={round:2,item_id:'scanned',price:700};
  const priced={round:2,item_id:'largest',price:1500,top_left:{x:4,y:0},bottom_right:{x:8,y:4}};
  const raw={...source,settlement:undefined,items:source.settlement.items,public_hints:[],initial_inspections:{p1:[inspected],p2:[]},item_reveals:{p1:[shape,price,priced],p2:[]},frames:[]};
  raw.frames=[{...raw,item_reveals:{p1:[],p2:[]}},{...raw,item_reveals:{p1:[shape],p2:[]}}];
  const observed=normalizeView(raw,true);
  assert.deepEqual(knownItems(observation(observed,0),'p1'),[inspected]);
  const scanned=knownItems(observation(observed,1),'p1').find(item=>item.id==='scanned');
  assert.equal(scanned.price,undefined);assert.equal(scanned.quality,undefined);
  assert.equal(knownItems(observed,'p1').find(item=>item.id==='scanned').price,700);
  assert.deepEqual(knownItems(observed,'p1').find(item=>item.id==='largest'),{id:'largest',price:1500,top_left:priced.top_left,bottom_right:priced.bottom_right});
  assert.deepEqual(knownItems(observed,'p2'),[]);
  const html=renderContent(observed,emptyStats,{...ui,cursor:null,observer:true});
  assert.ok(html.includes('data-item="scanned"')&&html.includes('data-quality="unrevealed"'));
  assert.ok(html.includes('data-item="largest" data-quality="unrevealed"'));
  assert.ok(itemInfo(observation(observed,null),{...ui,focusItem:'largest',observer:true}).includes('1,500'));
  const detail=itemInfo(observation(observed,null),{...ui,focusItem:'scanned',observer:true});
  assert.ok(detail.includes('700'));
  const identified={...observed,item_reveals:{...observed.item_reveals,p1:[...observed.item_reveals.p1,{round:3,item_id:'scanned',quality:'epic'}]}};
  const identifiedHTML=renderContent(identified,emptyStats,{...ui,cursor:null,observer:true});
  assert.ok(identifiedHTML.includes('data-item="scanned" data-quality="epic"'));
  assert.equal(knownItems(identified,'p1').find(item=>item.id==='scanned').price,700);
  const expired=normalizeView(observed,false);
  assert.deepEqual(knownItems(expired,'p1'),[]);
  assert.deepEqual(knownItems(observation(expired,1),'p1'),[]);
  const merged=knownItems({...observed,public_hints:[{data:{fact:{type:'random_quality',item_id:'largest',quality:'epic'}}}]},'p1').find(item=>item.id==='largest');
  assert.equal(merged.quality,'epic');assert.equal(merged.price,1500);
});
