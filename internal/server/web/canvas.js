// Camera state belongs to the room, not to a replay frame or an SSE update.
const cameras=new Map(),mounted=new WeakSet(),pitch=244,gap=24;
let current,zooming,zoomFrame;
const reduced=matchMedia('(prefers-reduced-motion:reduce)');
const clamp=(value,min=.06)=>Math.max(min,Math.min(2.5,value));
const zoomDuration=180;
function stopZoom(){if(zoomFrame)cancelAnimationFrame(zoomFrame);zoomFrame=0;zooming=null;}
// Read the existing CSS curve rather than introducing a separate motion token.
function easing(css){
  const [x1,y1,x2,y2]=css.match(/[\d.]+/g).map(Number);
  const cubic=(a,b,t)=>3*(1-t)**2*t*a+3*(1-t)*t*t*b+t**3;
  return progress=>{
    if(progress<=0||progress>=1)return progress;
    let low=0,high=1;
    for(let i=0;i<14;i++){const mid=(low+high)/2;if(cubic(x1,x2,mid)<progress)low=mid;else high=mid;}
    return cubic(y1,y2,(low+high)/2);
  };
}
function paint(){
  if(!current)return;
  const {viewport,world,camera}=current;
  world.style.transform='translate('+camera.x+'px,'+camera.y+'px) scale('+camera.zoom+')';
  // Background properties do not inherit through the entire item tree.
  viewport.style.backgroundPosition=camera.x+'px '+camera.y+'px';
  viewport.style.backgroundSize=Math.max(12,32*camera.zoom)+'px '+Math.max(12,32*camera.zoom)+'px';
  const label=Math.round(camera.zoom*100)+'%';
  current.outputs.forEach(node=>{if(node.textContent!==label)node.textContent=label;});
}
function moveZoom(value,x,y){
  const c=current.camera,z=clamp(value,c.minZoom),ratio=z/c.zoom;
  c.x=x-(x-c.x)*ratio;c.y=y-(y-c.y)*ratio;c.zoom=z;
}
function zoomAt(value,x,y,animate=false,direction=0){
  stopZoom();
  const from=current.camera.zoom,target=clamp(value,current.camera.minZoom);
  if(!animate||reduced.matches||target===from){moveZoom(target,x,y);paint();return;}
  zooming={camera:current.camera,target,from,x,y,direction,start:performance.now()};
  const tick=time=>{
    if(!zooming||current?.camera!==zooming.camera){stopZoom();return;}
    const progress=Math.max(0,Math.min(1,(time-zooming.start)/zoomDuration));
    moveZoom(zooming.from*Math.exp(Math.log(zooming.target/zooming.from)*current.ease(progress)),zooming.x,zooming.y);
    paint();
    if(progress<1)zoomFrame=requestAnimationFrame(tick);else{zoomFrame=0;zooming=null;}
  };
  zoomFrame=requestAnimationFrame(tick);
}
const zoomBase=direction=>zooming?.direction===direction?zooming.target:current.camera.zoom;
function itemBounds(world){
  const items=[...world.querySelectorAll('[data-item]')];
  if(!items.length)return {left:0,top:0,right:world.offsetWidth,bottom:world.offsetHeight};
  return {
    left:Math.min(...items.map(item=>item.offsetLeft)),top:Math.min(...items.map(item=>item.offsetTop)),
    right:Math.max(...items.map(item=>item.offsetLeft+item.offsetWidth)),bottom:Math.max(...items.map(item=>item.offsetTop+item.offsetHeight))
  };
}
function fitArea(viewport){
  const padding=24,box=viewport.getBoundingClientRect();
  const area={left:padding,top:padding,right:viewport.clientWidth-padding,bottom:viewport.clientHeight-padding};
  const reserve=(selector,edge)=>{
    for(const node of viewport.closest('.auction-world').querySelectorAll(selector)){
      const rect=node.getBoundingClientRect();
      if(!rect.width||!rect.height||getComputedStyle(node).visibility==='hidden')continue;
      const side=edge||(rect.width<viewport.clientWidth*.75&&rect.left-box.left>=viewport.clientWidth/2?'right':'bottom');
      if(side==='top')area.top=Math.max(area.top,rect.bottom-box.top+padding);
      else if(side==='right')area.right=Math.min(area.right,rect.left-box.left-padding);
      else area.bottom=Math.min(area.bottom,rect.top-box.top-padding);
    }
  };
  reserve('.room-heading,.room-phase,.room-menu,.room-timeline,.vision-label','top');
  reserve('.world-seats,.room-dock,.canvas-tools','bottom');
  reserve('.room-inspector');
  return area;
}
export function canvasAction(action,animate=!document.body.classList.contains('using-keyboard')){
  if(!current)return;
  const {viewport,camera,world}=current;
  const width=viewport.clientWidth,height=viewport.clientHeight;
  camera.touched=true;
  if(action==='fit-box'){
    stopZoom();
    const bounds=itemBounds(world),area=fitArea(viewport);
    if(area.right<=area.left||area.bottom<=area.top)return;
    // Fit may go below the wheel minimum when needed to show every visible item.
    const zoom=Math.min(2.5,(area.right-area.left)/(bounds.right-bounds.left),(area.bottom-area.top)/(bounds.bottom-bounds.top));
    camera.zoom=zoom;camera.minZoom=Math.min(.06,zoom);
    camera.x=(area.left+area.right-(bounds.left+bounds.right)*zoom)/2;
    camera.y=(area.top+area.bottom-(bounds.top+bounds.bottom)*zoom)/2;paint();
  }else{
    const direction=action==='zoom-in'?1:-1;
    zoomAt((animate?zoomBase(direction):camera.zoom)*(direction===1?1.2:1/1.2),width/2,height/2,animate,direction);
  }
}
export function syncCanvas(){
  const viewport=document.querySelector('.canvas-viewport');
  if(!viewport){stopZoom();current=null;return;}
  const world=viewport.querySelector('.canvas-world');
  const width=Number(world.dataset.columns)*pitch-gap,height=Number(world.dataset.rows)*pitch-gap;
  world.style.width=width+'px';world.style.height=height+'px';
  world.querySelectorAll('[data-item]').forEach(item=>{
    item.style.left=Number(item.dataset.col)*pitch+'px';item.style.top=Number(item.dataset.row)*pitch+'px';
    item.style.width=Number(item.dataset.w)*pitch-gap+'px';item.style.height=Number(item.dataset.h)*pitch-gap+'px';
  });
  const key=viewport.dataset.room+'/'+viewport.dataset.vision,items=[...world.querySelectorAll('[data-item]')];
  const centerContent=camera=>{
    const {left,top,right,bottom}=itemBounds(world);
    if(items.length<=5)camera.zoom=clamp(Math.min(camera.zoom,(viewport.clientWidth-64)/(right-left),(viewport.clientHeight-300)/(bottom-top)));
    camera.x=viewport.clientWidth/2-(left+right)/2*camera.zoom;
    camera.y=viewport.clientHeight/2-40-(top+bottom)/2*camera.zoom;
  };
  let camera=cameras.get(key);
  if(!camera){
    const zoom=viewport.clientWidth<700?.45:Math.min(1,viewport.clientHeight/1000);
    camera={x:(viewport.clientWidth-width*zoom)/2,y:(viewport.clientHeight-height*zoom)/2-40,zoom,minZoom:.06,viewWidth:viewport.clientWidth,viewHeight:viewport.clientHeight};
    cameras.set(key,camera);
  }
  if(items.length&&!camera.hasContent){if(!camera.touched)centerContent(camera);camera.hasContent=true;}
  camera.x+=(viewport.clientWidth-camera.viewWidth)/2;camera.y+=(viewport.clientHeight-camera.viewHeight)/2;
  camera.viewWidth=viewport.clientWidth;camera.viewHeight=viewport.clientHeight;
  if(current&&(current.camera!==camera||current.viewport!==viewport))stopZoom();
  const ease=current?.viewport===viewport?current.ease:easing(getComputedStyle(viewport).getPropertyValue('--ease-out'));
  current={viewport,world,camera,ease,outputs:[...document.querySelectorAll('.zoom-value')]};
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
    stopZoom();
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
    if(points.size>1&&previous.distance)moveZoom(current.camera.zoom*next.distance/previous.distance,previous.x,previous.y);
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
    // d3-zoom's unit normalization also accounts for trackpad pinch (Ctrl+wheel).
    const delta=-event.deltaY*(event.deltaMode===1?.05:event.deltaMode===2?1:.002)*(event.ctrlKey?10:1);
    if(!delta)return;
    const box=viewport.getBoundingClientRect(),direction=Math.sign(delta);
    zoomAt(zoomBase(direction)*2**Math.max(-.25,Math.min(.25,delta)),event.clientX-box.left,event.clientY-box.top,true,direction);
  },{passive:false});
  viewport.addEventListener('keydown',event=>{
    if(event.target!==viewport)return;
    if(['+','=','-','Home','ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(event.key)){
      event.preventDefault();current.camera.touched=true;
      if(event.key==='Home')canvasAction('fit-box',false);
      else if(['+','=','-'].includes(event.key))canvasAction(event.key==='-'?'zoom-out':'zoom-in',false);
      else{stopZoom();current.camera.x+=event.key==='ArrowLeft'?60:event.key==='ArrowRight'?-60:0;current.camera.y+=event.key==='ArrowUp'?60:event.key==='ArrowDown'?-60:0;paint();}
    }
  });
  viewport.addEventListener('focusin',event=>{
    if(!document.body.classList.contains('using-keyboard')||!current||current.viewport!==viewport)return;
    const item=event.target.closest('[data-item]');if(!item)return;
    stopZoom();
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
  stopZoom();
  camera.x+=(viewport.clientWidth-camera.viewWidth)/2;camera.y+=(viewport.clientHeight-camera.viewHeight)/2;
  camera.viewWidth=viewport.clientWidth;camera.viewHeight=viewport.clientHeight;paint();
});
reduced.addEventListener('change',()=>{
  if(!reduced.matches||!zooming)return;
  moveZoom(zooming.target,zooming.x,zooming.y);stopZoom();paint();
});
document.addEventListener('visibilitychange',()=>{if(document.hidden)stopZoom();});
