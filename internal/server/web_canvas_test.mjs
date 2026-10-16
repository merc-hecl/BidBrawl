import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';

// Controlled DOM and animation clock for camera interaction unit tests.
const css=await readFile(new URL('./web/style.css',import.meta.url),'utf8');
const curve=css.match(/--ease-out:([^;]+)/)[1];
let clock=0,serial=0,view,keyboardMode=false;
const frames=new Map(),preferences=[];
const reduced={matches:false,addEventListener:(_,fn)=>preferences.push(fn)};
globalThis.matchMedia=()=>reduced;
globalThis.performance={now:()=>clock};
globalThis.requestAnimationFrame=fn=>{const id=++serial;frames.set(id,fn);return id;};
globalThis.cancelAnimationFrame=id=>frames.delete(id);
globalThis.getComputedStyle=node=>({getPropertyValue:()=>curve,visibility:node.hidden?'hidden':'visible'});
globalThis.addEventListener=()=>{};
globalThis.document={querySelector:()=>view,querySelectorAll:()=>view?[view.output]:[],getElementById:()=>({hidden:false}),addEventListener:()=>{},body:{classList:{contains:()=>keyboardMode}}};
const {syncCanvas,canvasAction}=await import('./web/canvas.js');
function camera(){const [x,y,zoom]=view.world.style.transform.match(/-?[\d.]+/g).map(Number);return {x,y,zoom};}
const near=(a,b)=>assert.ok(Math.abs(a-b)<1e-8,`${a} != ${b}`);
function frame(time){clock=time;const pending=[...frames.values()];frames.clear();for(const fn of pending)fn(time);}
function finish(){for(let i=0;frames.size&&i<40;i++)frame(clock+16);assert.equal(frames.size,0);}
function fixture({items=[],width=1200,height=800,hud=[]}={}){
  view=null;syncCanvas();clock=0;reduced.matches=false;keyboardMode=false;
  const blocks=items.map(data=>{
    const item={style:{},dataset:{col:0,row:0,w:1,h:1,...data}};
    for(const [property,style] of Object.entries({offsetLeft:'left',offsetTop:'top',offsetWidth:'width',offsetHeight:'height'}))Object.defineProperty(item,property,{get:()=>parseFloat(item.style[style])});
    return item;
  });
  const overlays=hud.map(({kind,left=0,top=0,width,height,hidden=false})=>({kind,hidden,getBoundingClientRect:()=>({left,top,width,height,right:left+width,bottom:top+height})}));
  const root={querySelectorAll:selector=>overlays.filter(node=>selector.split(',').includes('.'+node.kind))};
  const handlers=new Map(),world={style:{},dataset:{columns:'20',rows:'30'},querySelectorAll:()=>blocks};
  Object.defineProperties(world,{offsetWidth:{get:()=>parseFloat(world.style.width)},offsetHeight:{get:()=>parseFloat(world.style.height)}});
  view={world,style:{},output:{textContent:''},dataset:{room:'room-'+ ++serial,vision:'public'},clientWidth:width,clientHeight:height,querySelector:()=>world,getBoundingClientRect:()=>({left:0,top:0}),addEventListener:(type,fn)=>handlers.set(type,fn),classList:{add(){},remove(){}},closest:selector=>selector==='.auction-world'?root:null,setPointerCapture(){}};
  view.emit=(type,values={})=>handlers.get(type)({button:0,target:view,preventDefault(){},...values});
  view.wheel=(deltaY,values={})=>view.emit('wheel',{deltaY,deltaMode:0,clientX:300,clientY:250,...values});
  syncCanvas();return view;
}

test('wheel zoom advances across frames while keeping the cursor anchor fixed',()=>{
  const v=fixture(),before=camera(),samples=[];
  const worldPoint={x:(300-before.x)/before.zoom,y:(250-before.y)/before.zoom};
  v.wheel(-120);
  near(camera().zoom,before.zoom);
  for(const time of [16,32,64,96,180]){
    frame(time);const c=camera();samples.push(c.zoom);
    near(c.x+worldPoint.x*c.zoom,300);near(c.y+worldPoint.y*c.zoom,250);
  }
  assert.ok(samples.every((zoom,i)=>zoom>(i?samples[i-1]:before.zoom)));
  near(camera().zoom,before.zoom*2**.24);
  assert.equal(frames.size,0);
});

test('rapid input accumulates, reversal responds immediately and changing anchors does not jump',()=>{
  const v=fixture(),initial=camera();
  v.wheel(-100);v.wheel(-100);finish();
  near(camera().zoom,initial.zoom*2**.4);
  v.wheel(-120);frame(clock+32);
  const before=camera(),anchor={x:(700-before.x)/before.zoom,y:(500-before.y)/before.zoom};
  v.wheel(120,{clientX:700,clientY:500});
  assert.deepEqual(camera(),before);
  frame(clock+16);assert.ok(camera().zoom<before.zoom);
  near(camera().x+anchor.x*camera().zoom,700);near(camera().y+anchor.y*camera().zoom,500);
  finish();
});

test('elapsed-time interpolation behaves the same at different frame rates',()=>{
  const v=fixture();v.wheel(-120);for(const time of [20,40,60])frame(time);const slower=camera();
  const fast=fixture();fast.wheel(-120);for(const time of [10,20,30,40,50,60])frame(time);const faster=camera();
  near(slower.zoom,faster.zoom);near(slower.x,faster.x);near(slower.y,faster.y);
});

test('wheel units, trackpad pinch, reduced motion and scale limits are respected',()=>{
  const pixel=fixture();reduced.matches=true;pixel.wheel(-100);const pixelZoom=camera().zoom;
  const line=fixture();reduced.matches=true;line.wheel(-4,{deltaMode:1});near(camera().zoom,pixelZoom);
  const pinch=fixture();reduced.matches=true;pinch.wheel(-10,{ctrlKey:true});near(camera().zoom,pixelZoom);
  for(let i=0;i<100;i++)pinch.wheel(-10000);
  near(camera().zoom,2.5);
  for(let i=0;i<100;i++)pinch.wheel(10000);
  near(camera().zoom,.06);assert.equal(frames.size,0);
  reduced.matches=false;pinch.wheel(-120);frame(clock+16);reduced.matches=true;preferences.forEach(fn=>fn());
  assert.equal(frames.size,0);
});

test('redraw preserves zoom in flight while room/vision switching cancels and retains visible position',()=>{
  const v=fixture();v.wheel(-120);frame(32);const before=camera();
  syncCanvas();assert.deepEqual(camera(),before);assert.equal(frames.size,1);
  v.dataset.vision='p1';syncCanvas();const player=camera();assert.equal(frames.size,0);
  frame(200);assert.deepEqual(camera(),player);
  v.dataset.vision='public';syncCanvas();assert.deepEqual(camera(),before);
  view=null;syncCanvas();assert.equal(frames.size,0);
});

test('dragging and keyboard input interrupt pending zoom; pinch stays directly attached to fingers',()=>{
  const v=fixture();v.wheel(-120);frame(32);const before=camera();
  v.emit('pointerdown',{pointerId:1,clientX:300,clientY:250});
  v.emit('pointermove',{pointerId:1,clientX:330,clientY:270});
  assert.equal(frames.size,0);near(camera().zoom,before.zoom);near(camera().x,before.x+30);
  v.emit('pointerdown',{pointerId:2,clientX:430,clientY:270});
  v.emit('pointermove',{pointerId:2,clientX:530,clientY:270});
  near(camera().zoom,before.zoom*2);assert.equal(frames.size,0);
  v.emit('pointerup',{pointerId:1});v.emit('pointerup',{pointerId:2});
  v.wheel(100);frame(clock+16);
  const keyboard=camera();v.emit('keydown',{key:'+'});
  near(camera().zoom,keyboard.zoom*1.2);assert.equal(frames.size,0);
  const positioned=camera();v.emit('keydown',{key:'ArrowLeft'});near(camera().x,positioned.x+60);
  keyboardMode=true;const button=camera();canvasAction('zoom-out');near(camera().zoom,button.zoom/1.2);assert.equal(frames.size,0);
  canvasAction('fit-box');assert.equal(frames.size,0);
});

function visibleBounds(v){
  const c=camera(),items=v.world.querySelectorAll();
  return {left:Math.min(...items.map(i=>i.offsetLeft))*c.zoom+c.x,
    top:Math.min(...items.map(i=>i.offsetTop))*c.zoom+c.y,
    right:Math.max(...items.map(i=>i.offsetLeft+i.offsetWidth))*c.zoom+c.x,
    bottom:Math.max(...items.map(i=>i.offsetTop+i.offsetHeight))*c.zoom+c.y};
}
test('fit fills the usable height with actual offset items and excludes unused box rows',()=>{
  const v=fixture({items:[{col:4,row:5,w:2,h:2},{col:9,row:9,w:3,h:4}],hud:[
    {kind:'room-timeline',top:88,width:300,height:40},
    {kind:'world-seats',top:640,width:300,height:130},
    {kind:'canvas-tools',top:718,width:200,height:52}
  ]});
  v.wheel(-100);frame(32);canvasAction('fit-box');
  const bounds=visibleBounds(v),c=camera();
  near(bounds.top,152);near(bounds.bottom,616);near((bounds.left+bounds.right)/2,600);
  assert.ok(bounds.left>=24&&bounds.right<=1176);
  assert.ok(c.zoom>2*(800-280)/v.world.offsetHeight);
  assert.equal(frames.size,0);
  v.emit('keydown',{key:'Home'});assert.deepEqual(camera(),c);
});
test('fit reserves an open side inspector and ignores hidden controls',()=>{
  const v=fixture({items:[{col:0,row:0,w:2,h:2},{col:10,row:0,w:2,h:2}],hud:[
    {kind:'room-heading',top:24,width:416,height:68},
    {kind:'room-inspector',left:772,top:112,width:400,height:500},
    {kind:'world-seats',top:400,width:500,height:300,hidden:true}
  ]});
  canvasAction('fit-box');const bounds=visibleBounds(v);
  near(bounds.left,24);near(bounds.right,748);
  assert.ok(bounds.top>=116&&bounds.bottom<=776);
});
test('fit can show large visible extents below the wheel minimum without jumping on the next zoom',()=>{
  const v=fixture({width:390,height:844,items:[{col:0,row:0,w:1,h:1},{col:19,row:29,w:1,h:1}],hud:[
    {kind:'room-timeline',top:146,width:200,height:38},
    {kind:'room-inspector',left:12,top:480,width:366,height:352}
  ]});
  canvasAction('fit-box');const c=camera(),bounds=visibleBounds(v);
  assert.ok(c.zoom<.06);
  near(bounds.top,208);near(bounds.bottom,456);
  assert.ok(bounds.left>=24&&bounds.right<=366);
  reduced.matches=true;v.wheel(-100);
  near(camera().zoom,c.zoom*2**.2);assert.ok(camera().zoom<.06);
  v.wheel(100);near(camera().zoom,c.zoom);
});
test('empty views fit the box and a single item respects the maximum zoom',()=>{
  const empty=fixture();canvasAction('fit-box');
  near(camera().zoom,(800-48)/empty.world.offsetHeight);
  const single=fixture({items:[{col:12,row:18,w:1,h:1}]});canvasAction('fit-box');
  near(camera().zoom,2.5);const bounds=visibleBounds(single);
  near((bounds.left+bounds.right)/2,600);near((bounds.top+bounds.bottom)/2,400);
});
