// Camera state belongs to the room, not to a replay frame or an SSE update.
const cameras=new Map(),mounted=new WeakSet(),pitch=244,gap=24;
let current;
const clamp=value=>Math.max(.06,Math.min(2.5,value));
function paint(){
  if(!current)return;
  const {viewport,world,camera}=current;
  world.style.transform='translate('+camera.x+'px,'+camera.y+'px) scale('+camera.zoom+')';
  viewport.style.setProperty('--camera-x',camera.x+'px');
  viewport.style.setProperty('--camera-y',camera.y+'px');
  viewport.style.setProperty('--dot-pitch',Math.max(12,32*camera.zoom)+'px');
  document.querySelectorAll('.zoom-value').forEach(node=>node.textContent=Math.round(camera.zoom*100)+'%');
}
function zoomAt(value,x,y){
  const c=current.camera,z=clamp(value),ratio=z/c.zoom;
  c.x=x-(x-c.x)*ratio;c.y=y-(y-c.y)*ratio;c.zoom=z;paint();
}
export function canvasAction(action){
  if(!current)return;
  const {viewport,camera,world}=current;
  const width=viewport.clientWidth,height=viewport.clientHeight;
  camera.touched=true;
  if(action==='fit-box'){
    const zoom=clamp(Math.min((width-64)/world.offsetWidth,(height-(width<700?260:280))/world.offsetHeight));
    camera.zoom=zoom;camera.x=(width-world.offsetWidth*zoom)/2;camera.y=(height-world.offsetHeight*zoom)/2-36;paint();
  }else zoomAt(camera.zoom*(action==='zoom-in'?1.2:1/1.2),width/2,height/2);
}
export function syncCanvas(){
  const viewport=document.querySelector('.canvas-viewport');
  if(!viewport){current=null;return;}
  const world=viewport.querySelector('.canvas-world');
  const width=Number(world.dataset.columns)*pitch-gap,height=Number(world.dataset.rows)*pitch-gap;
  world.style.width=width+'px';world.style.height=height+'px';
  world.querySelectorAll('[data-item]').forEach(item=>{
    item.style.left=Number(item.dataset.col)*pitch+'px';item.style.top=Number(item.dataset.row)*pitch+'px';
    item.style.width=Number(item.dataset.w)*pitch-gap+'px';item.style.height=Number(item.dataset.h)*pitch-gap+'px';
  });
  const key=viewport.dataset.room+'/'+viewport.dataset.vision,items=[...world.querySelectorAll('[data-item]')];
  const centerContent=camera=>{
    const left=Math.min(...items.map(item=>item.offsetLeft)),top=Math.min(...items.map(item=>item.offsetTop));
    const right=Math.max(...items.map(item=>item.offsetLeft+item.offsetWidth)),bottom=Math.max(...items.map(item=>item.offsetTop+item.offsetHeight));
    if(items.length<=5)camera.zoom=clamp(Math.min(camera.zoom,(viewport.clientWidth-64)/(right-left),(viewport.clientHeight-300)/(bottom-top)));
    camera.x=viewport.clientWidth/2-(left+right)/2*camera.zoom;
    camera.y=viewport.clientHeight/2-40-(top+bottom)/2*camera.zoom;
  };
  let camera=cameras.get(key);
  if(!camera){
    const zoom=viewport.clientWidth<700?.45:Math.min(1,viewport.clientHeight/1000);
    camera={x:(viewport.clientWidth-width*zoom)/2,y:(viewport.clientHeight-height*zoom)/2-40,zoom,viewWidth:viewport.clientWidth,viewHeight:viewport.clientHeight};
    cameras.set(key,camera);
  }
  if(items.length&&!camera.hasContent){if(!camera.touched)centerContent(camera);camera.hasContent=true;}
  camera.x+=(viewport.clientWidth-camera.viewWidth)/2;camera.y+=(viewport.clientHeight-camera.viewHeight)/2;
  camera.viewWidth=viewport.clientWidth;camera.viewHeight=viewport.clientHeight;
  current={viewport,world,camera};
  paint();
  if(mounted.has(viewport))return;
  mounted.add(viewport);
  const points=new Map();let previous,travel=0,suppressClick=false;
  const gesture=()=>{
    const p=[...points.values()];
    return {x:p.reduce((s,p)=>s+p.x,0)/p.length,y:p.reduce((s,p)=>s+p.y,0)/p.length,distance:p.length>1?Math.hypot(p[1].x-p[0].x,p[1].y-p[0].y):0};
  };
  viewport.addEventListener('pointerdown',event=>{
    if(event.button!==0||!current||current.viewport!==viewport)return;
    const box=viewport.getBoundingClientRect();
    points.set(event.pointerId,{x:event.clientX-box.left,y:event.clientY-box.top});
    (event.target.closest('button')||viewport).setPointerCapture(event.pointerId);
    previous=gesture();if(points.size===1){travel=0;suppressClick=false;}
    viewport.classList.add('is-panning');document.getElementById('item-tooltip').hidden=true;
  });
  viewport.addEventListener('pointermove',event=>{
    if(!points.has(event.pointerId)||!current||current.viewport!==viewport)return;
    const box=viewport.getBoundingClientRect();
    points.set(event.pointerId,{x:event.clientX-box.left,y:event.clientY-box.top});
    const next=gesture(),dx=next.x-previous.x,dy=next.y-previous.y;
    travel+=Math.abs(dx)+Math.abs(dy);if(travel>5||points.size>1)current.camera.touched=true;
    if(points.size>1&&previous.distance)zoomAt(current.camera.zoom*next.distance/previous.distance,previous.x,previous.y);
    current.camera.x+=dx;current.camera.y+=dy;previous=next;
    suppressClick=travel>5||points.size>1;paint();
  });
  const end=event=>{
    points.delete(event.pointerId);if(points.size)previous=gesture();else viewport.classList.remove('is-panning');
  };
  viewport.addEventListener('pointerup',end);viewport.addEventListener('pointercancel',end);
  viewport.addEventListener('lostpointercapture',end);
  viewport.addEventListener('click',event=>{if(suppressClick){event.preventDefault();event.stopPropagation();suppressClick=false;}},true);
  viewport.addEventListener('wheel',event=>{
    if(!current||current.viewport!==viewport)return;
    event.preventDefault();current.camera.touched=true;document.getElementById('item-tooltip').hidden=true;
    const box=viewport.getBoundingClientRect(),delta=event.deltaY*(event.deltaMode===1?16:event.deltaMode===2?viewport.clientHeight:1);
    zoomAt(current.camera.zoom*Math.exp(-Math.max(-200,Math.min(200,delta))*.003),event.clientX-box.left,event.clientY-box.top);
  },{passive:false});
  viewport.addEventListener('keydown',event=>{
    if(event.target!==viewport)return;
    if(['+','=','-','Home','ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(event.key)){
      event.preventDefault();current.camera.touched=true;
      if(event.key==='Home')canvasAction('fit-box');
      else if(['+','=','-'].includes(event.key))canvasAction(event.key==='-'?'zoom-out':'zoom-in');
      else{current.camera.x+=event.key==='ArrowLeft'?60:event.key==='ArrowRight'?-60:0;current.camera.y+=event.key==='ArrowUp'?60:event.key==='ArrowDown'?-60:0;paint();}
    }
  });
  viewport.addEventListener('focusin',event=>{
    if(!document.body.classList.contains('using-keyboard')||!current||current.viewport!==viewport)return;
    const item=event.target.closest('[data-item]');if(!item)return;
    viewport.scrollLeft=0;viewport.scrollTop=0;
    const c=current.camera,x=item.offsetLeft*c.zoom+c.x,y=item.offsetTop*c.zoom+c.y;
    if(x<24||y<140||x+item.offsetWidth*c.zoom>viewport.clientWidth-24||y+item.offsetHeight*c.zoom>viewport.clientHeight-190){
      c.x=viewport.clientWidth/2-(item.offsetLeft+item.offsetWidth/2)*c.zoom;
      c.y=viewport.clientHeight/2-40-(item.offsetTop+item.offsetHeight/2)*c.zoom;paint();
    }
  });
}
addEventListener('resize',()=>{
  if(!current)return;
  const {viewport,camera}=current;
  camera.x+=(viewport.clientWidth-camera.viewWidth)/2;camera.y+=(viewport.clientHeight-camera.viewHeight)/2;
  camera.viewWidth=viewport.clientWidth;camera.viewHeight=viewport.clientHeight;paint();
});
