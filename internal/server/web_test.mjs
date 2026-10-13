import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {catalogs,t,setLocale,localizeCard} from './web/i18n.js';
import {normalizeView,observation,knownItems} from './web/model.js';
import {renderContent,roomDirectory,dialogContent} from './web/render.js';

const emptyStats={games:0,profit:0,paid:0,rate:null,agents:[]};
// Fixture frames represent the server projection consumed by the renderer.
function withFrames(s){return {...s,frames:Array.from({length:s.history.length+(s.settlement?2:1)},(_,i)=>({...s,phase:s.settlement&&i>=s.history.length?'settled':'bidding',round:Math.min(i+1,s.round),history:s.history.slice(0,i),public_hints:s.public_hints.filter(h=>h.round<=i+1),decisions:s.decisions.filter(d=>d.round<=i),settlement:i>=s.history.length?s.settlement:undefined}))};}
const ui={room:'r',page:'room',cursor:2,sort:'profit',quality:'all',search:'',focusPlayer:'p1'};
let source={room_id:'r',room_name:'<Room>',phase:'settled',round:5,rules:{rounds:5,box_width:20,box_height:30,player_count:2},players:[{id:'p1',name:'<Atlas>',avatar:5,submitted:true},{id:'p2',name:'Echo',avatar:1,submitted:true}],history:Array.from({length:5},(_,i)=>({round:i+1,bids:[{player_id:'p1',amount:100+i},{player_id:'p2',amount:200+i}]})),public_hints:[{round:1,kind:'random_shape',data:{title:'随机物品轮廓',message:'物品 known 占 1 × 1 格。',fact:{type:'random_shape',item_id:'known',top_left:{x:0,y:0},bottom_right:{x:1,y:1}},translations:{en:{title:'Random item shape',message:'Item known occupies 1 × 1 cells.'}}}},{round:3,kind:'quality_count',data:{title:'指定品质数量',message:'箱内稀有品质物品共有 2 件。',fact:{type:'quality_count',quality:'rare',count:2}}}],settlement:{winner_id:'p2',paid:204,total_value:300,profit:96,tied_players:['p2'],items:[{id:'known',quality:'rare',price:100,top_left:{x:0,y:0},bottom_right:{x:1,y:1}},{id:'hidden-item',quality:'common',price:200,top_left:{x:1,y:0},bottom_right:{x:2,y:1}}],round:5,method:'final',payoffs:{p1:0,p2:96}},decisions:[]};

source=withFrames(source);

test('room directory is empty until creation and management is restricted',()=>{
  for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    assert.ok(roomDirectory([],{manage:true}).includes('data-action="create-room"'));
    assert.ok(!roomDirectory([],{manage:false}).includes('data-action="create-room"'));
    const rooms=roomDirectory([{room_id:'r',name:'<Table>',phase:'lobby',players:1,player_count:3}],{manage:true});
    assert.ok(rooms.includes('&lt;Table&gt;')&&rooms.includes('data-action="room:r"'));
    assert.ok(dialogContent('create-room',null,null,emptyStats,ui).content.includes('name="players"'));
    assert.ok(dialogContent('invite-room','http://localhost/invite?room=r',null,emptyStats,ui).content.includes('bidbrawl join -invite'));
  }
});

test('statistics and agent profile work without a selected room and never filter by it',()=>{
  const results={games:1,profit:-15,paid:100,rate:-15,agents:[{id:'account:'+'a'.repeat(32),name:'Moss',games:1,profit:-15,paid:100,rate:-15,average:-15,max_profit:0,max_loss:-15}]};
  const html=renderContent(null,results,{...ui,page:'stats',room:'unrelated'});
  assert.ok(html.includes('Moss')&&!html.includes('data-control="scope"'));
  const profile=dialogContent('stats-agent','account:'+ 'a'.repeat(32),null,results,ui);
  assert.ok(profile.content.includes('class="agent-identity"')&&profile.content.includes('class="agent-metrics"'));
  assert.ok(!profile.content.includes('account:')&&!profile.content.includes('<dl>'));
});
source.rules.cards=JSON.parse(await readFile(new URL('../cards/default.json',import.meta.url),'utf8')).cards;

test('approved public facts reveal shapes and merge same-item quality and price',()=>{
  const view={...source,settlement:undefined,public_hints:[
    {round:1,data:{fact:{type:'random_shape',item_id:'known',top_left:{x:0,y:0},bottom_right:{x:1,y:1}}}},
    {round:3,data:{fact:{type:'random_quality_price',item_id:'known',quality:'rare',price:456}}},
    {round:3,data:{fact:{type:'quality_count',quality:'common',count:5}}},
    {round:3,data:{fact:{type:'random_quality_price',item_id:'unlocated',quality:'epic',price:999}}},
  ]};
  assert.deepEqual(knownItems(view),[{id:'known',top_left:{x:0,y:0},bottom_right:{x:1,y:1},quality:'rare',price:456}]);
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
  assert.ok(en.includes('total item count'));
  setLocale('zh-CN',false);
});

test('locales have matching keys, placeholders and no untranslated renderer text',async()=>{
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
  for(const file of ['app.js','render.js','room-ui.js','avatars.js','appearance.js','model.js','style.css']) {
    const text=await readFile(new URL('./web/'+file,import.meta.url),'utf8');
    const withoutComments=text.replace(/\/\*[\s\S]*?\*\//g,'').replace(/\/\/[^\n]*/g,'');
    assert.equal(/[\u4e00-\u9fff]/.test(withoutComments),false,file);
  }
  setLocale('zh-CN',false);assert.equal(t('round',{round:3}),'第 3 轮');
  setLocale('en',false);assert.equal(t('round',{round:3}),'Round 3');
  assert.equal(t('gamesCount',{count:1}),'1 game');assert.equal(t('gamesCount',{count:2}),'2 games');
  assert.throws(()=>t('missing-key'));
  const builtin=JSON.parse(await readFile(new URL('../cards/default.json',import.meta.url),'utf8')).cards;
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
  assert.ok(!html.includes('hidden-item'));assert.ok(!html.includes('nonce'));
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
  assert.equal('nonce' in early.settlement,false);assert.equal('commitment' in early,false);
});

test('player pane shows only selected player and round reasons and explains restricted access',()=>{
  const recorded=withFrames({...source,decisions:[
    {round:1,player_id:'p1',action:'bid',reason:'first-round-rationale'},
    {round:2,player_id:'p2',action:'card',card_id:'item_count',reason:'second-round-rationale'}
  ]});
  const first=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:1,observer:true});
  assert.ok(first.includes(`<span class="status-pill world-phase"><i></i>${t('round',{round:1})}</span>`));
  assert.ok(first.includes('first-round-rationale'));
  assert.ok(!first.includes('second-round-rationale'));
  assert.ok(first.includes('data-action="bids"'));
  const second=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:2,observer:true,focusPlayer:'p2'});
  assert.ok(second.includes('second-round-rationale'));
  assert.ok(!second.includes('first-round-rationale'));
  const final=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:6,observer:true});
  assert.ok(!final.includes('first-round-rationale')&&!final.includes('second-round-rationale'));
  const opening=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:0,observer:true});
  assert.ok(opening.includes(t('openingDecision'))&&!opening.includes('first-round-rationale'));
  assert.ok(opening.includes(`<span class="status-pill world-phase"><i></i>${t('opening')}</span>`));
  const latest=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:null,observer:true});
  assert.ok(!latest.includes('first-round-rationale')&&!latest.includes('second-round-rationale'));
  assert.ok(latest.includes(`<span class="status-pill world-phase"><i></i>${t('settled')}</span>`));
  const anonymous=renderContent(recorded,emptyStats,{...ui,page:'room',cursor:1,observer:false});
  assert.ok(anonymous.includes(t('openFromHost'))&&!anonymous.includes('data-action="observer-login"'));
  assert.ok(!anonymous.includes('first-round-rationale')&&!anonymous.includes('second-round-rationale'));
});

test('observer skill names and help use the saved card definition safely in both languages',()=>{
  const played=withFrames({...source,decisions:[{round:1,player_id:'p1',action:'card',card_id:'value_band',reason:'estimate'}]});
  for(const locale of ['zh-CN','en']){
    setLocale(locale,false);
    const html=renderContent(played,emptyStats,{...ui,page:'room',cursor:1,observer:true});
    assert.ok(html.includes(t('card.value_band.name'))&&html.includes(t('card.value_band.description')));
    assert.ok(html.includes('data-action="card:value_band"')&&html.includes('role="tooltip"'));
    const detail=dialogContent('card','value_band',played,emptyStats,{...ui,observer:true});
    assert.equal(detail.title,t('card.value_band.name'));
    assert.ok(detail.content.includes(t('card.value_band.description')));
    assert.equal(dialogContent('card','value_band',played,emptyStats,{...ui,observer:false}),null);
  }
  const custom=withFrames({...played,rules:{...source.rules,cards:[{id:'custom',name:'<Card>',description:'<img src=x onerror=alert(1)>',effect:{type:'item_count'}}]},decisions:[{round:1,player_id:'p1',action:'card',card_id:'custom',reason:'estimate'}]});
  const html=renderContent(custom,emptyStats,{...ui,page:'room',cursor:1,observer:true});
  assert.ok(html.includes('&lt;Card&gt;')&&html.includes('&lt;img src=x onerror=alert(1)&gt;'));
  assert.ok(!html.includes('<img src=x'));
  const anonymous=renderContent(custom,emptyStats,{...ui,page:'room',cursor:1,observer:false});
  assert.ok(!anonymous.includes('Card')&&!anonymous.includes('onerror'));
});


test('room uses persisted avatars and fitted item bounds without zoom controls',()=>{
  const html=renderContent(source,emptyStats,{...ui,cursor:null});
  assert.ok(html.includes('data-bot="5"')&&html.includes('data-bot="1"'));
  assert.ok(html.includes('data-x="0" data-y="0" data-width="5" data-height="3.3333333333333335"'));
  assert.ok(html.includes('data-columns="20" data-rows="30"'));
  assert.equal((html.match(/data-item=/g)||[]).length,2);
  assert.ok(!html.includes('zoom'));
  assert.ok(!html.includes('style='));
  const stats=renderContent(null,emptyStats,{...ui,page:'stats'});
  assert.ok(stats.indexOf('class="metrics"')<stats.indexOf('data-region="scorecard"'));
  assert.ok(stats.indexOf('data-region="scorecard"')<stats.indexOf('data-control="sort"'));
});
