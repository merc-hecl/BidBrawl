import {initialLocale,setLocale,getLocale,t,translateDOM} from './i18n.js';
import {updateAppearance,closeMobileSidebar,greet} from './appearance.js';
import {icon} from './html.js';
import {itemTip} from './room-ui.js';
import {observation,knownItems,normalizeView} from './model.js';
import {esc,renderContent,roomDirectory,roomButtons,roomGroup,maxFrame,dialogContent} from './render.js';
import {observe} from './live.js';
import {patchContent} from './patch.js';
import {restoreSession} from './session.js';

const $=id=>document.getElementById(id);
const text=(id,value)=>{const node=$(id),next=String(value);if(node.textContent!==next)node.textContent=next;};
const initialRoom=new URLSearchParams(location.search).get('room')||'';
const ui={room:initialRoom,roomGroup:null,page:initialRoom?'room':'rooms',cursor:null,sort:'profit',quality:'all',search:'',observer:false,manage:false,focusPlayer:null,focusItem:null};
let source=null,rooms=[],results={games:0,profit:0,paid:0,rate:null,agents:[]},connection='connecting',resultsFailed=false;
let lastContent='',lastRooms='',lastContentObserver=false;
let managing=false,activeDialog=null;
const dialog=document.querySelector('dialog');
setLocale(initialLocale(),false);

function redraw() {
  const roomPending=!!source && source.room_id!==ui.room;
  const contentPending=roomPending && ui.page==='room';
  const navigationPage=ui.page==='room'?'rooms':ui.page;
  translateDOM();
  updateAppearance();
  $('language-button').setAttribute('aria-label',t('switchLanguage'));
  $('create-button').hidden=!ui.manage;
  document.querySelector('.app').dataset.view=ui.page;
  text('page-name',t(navigationPage));
  document.querySelectorAll('[data-page]').forEach(button=>{
    const selected=button.dataset.page===navigationPage;
    button.classList.toggle('selected',selected);
    if(selected)button.setAttribute('aria-current','page');else button.removeAttribute('aria-current');
  });
  text('connection',t(connection));
  $('connection').classList.toggle('offline',connection==='disconnected');
  $('error').hidden=connection!=='disconnected' && !(ui.page==='stats' && resultsFailed);
  text('error',t(connection==='disconnected'?'loadError':'resultsError'));
  const completedCount=rooms.filter(room=>roomGroup(room)==='completed').length;
  text('active-room-count',rooms.length-completedCount);
  text('completed-room-count',completedCount);
  document.querySelectorAll('[data-room-group]').forEach(button=>{
    button.setAttribute('aria-pressed',String(button.dataset.roomGroup===(ui.roomGroup||'active')));
  });
  $('rules-button').hidden=!source||roomPending||ui.page==='stats'||ui.page==='rooms';
  const side=roomButtons(rooms,ui.room,ui.roomGroup||'active');
  if(lastRooms!==side){patchContent($('rooms'),side);lastRooms=side;}
  // Keep the rendered room intact until its replacement snapshot arrives.
  const html=contentPending&&lastContentObserver===ui.observer?lastContent:ui.page==='rooms'?roomDirectory(rooms,ui):renderContent(source,results,ui);
  if(lastContent!==html){
    patchContent($('content'),html);lastContent=html;
  }
  fitCanvas();
  lastContentObserver=ui.observer;
  $('content').inert=contentPending;
  $('content').setAttribute('aria-busy',contentPending||connection==='connecting'?'true':'false');
}
function updateDialog() {
  if(!activeDialog)return;
  const content=dialogContent(activeDialog.kind,activeDialog.id,source,results,ui);
  if(!content){dialog.close();return;}
  patchContent(dialog,`<div class="dialog-head"><h2 id="dialog-title">${esc(content.title)}</h2><button class="dialog-close" aria-label="${esc(t('close'))}">${icon('close')}</button></div>${content.content}`);
  dialog.querySelector('button').onclick=()=>dialog.close();
}
function showDialog(kind,id) {
  ui.agentProfile=null;activeDialog={kind,id};updateDialog();if(activeDialog)dialog.showModal();
  if(kind==='agent'||kind==='stats-agent'){
    const identity=kind==='stats-agent'?id:source?.players.find(p=>p.id===id)?.agent_id;
    const request=activeDialog;
    if(identity)readJSON('/api/agents/'+encodeURIComponent(identity.replace(/^account:/,''))).then(profile=>{
      if(activeDialog===request){ui.agentProfile=profile;updateDialog();}
    }).catch(()=>{});
  }
}
dialog.addEventListener('close',()=>{activeDialog=null;});
function chooseRoom(id) {
  closeMobileSidebar();hideTip();ui.focusPlayer=null;ui.focusItem=null;
  ui.page='room';
  ui.roomGroup=roomGroup(rooms.find(room=>room.room_id===id));
  if(id===ui.room){redraw();return;}
  ui.room=id;ui.cursor=null;ui.quality='all';ui.search='';
  connection='connecting';
  dialog.close();
  const url=new URL(location.href);url.searchParams.set('room',id);history.replaceState(null,'',url);
  redraw();live.changeRoom();
}
document.addEventListener('click',event=>{
  const group=event.target.closest('[data-room-group]')?.dataset.roomGroup;
  if(group){ui.roomGroup=group;$('rooms').scrollTop=0;redraw();return;}
  const page=event.target.closest('[data-page]')?.dataset.page;
  if(page){event.preventDefault();closeMobileSidebar();hideTip();ui.page=page;ui.cursor=null;redraw();return;}
  const action=event.target.closest('[data-action]')?.dataset.action;
  if(!action)return;
  const separator=action.indexOf(':'),kind=separator<0?action:action.slice(0,separator),id=separator<0?'':action.slice(separator+1);
  if(kind==='avatar-greet'){greet(event.target.closest('button'));return;}
  if(kind==='focus-agent'){ui.focusPlayer=ui.focusPlayer===id?null:id;hideTip();redraw();greet(event.target.closest('button'));return;}
  if(kind==='dismiss-player'){const focused=ui.focusPlayer;ui.focusPlayer=null;redraw();document.querySelector(`[data-action="focus-agent:${CSS.escape(focused)}"]`)?.focus();return;}
  if(kind==='select-item'){ui.focusItem=ui.focusItem===id?null:id;hideTip();redraw();return;}
  if(kind==='unpin'){ui.focusItem=null;redraw();return;}
  if(['avatars','hints','inventory','bids'].includes(kind)){hideTip();showDialog(kind);return;}
  if(kind==='room'){chooseRoom(id);return;}
  if(kind==='create-room'){showDialog('create-room');return;}
  if(kind==='invite-room'||kind==='start-room'){manageRoom(kind);return;}
  if(kind==='copy-invite'){
    navigator.clipboard.writeText($('invite-link').value).then(()=>{event.target.textContent=t('copied');}).catch(()=>{event.target.textContent=t('copyFailed');});return;
  }
  if(['item','agent','stats-agent','card'].includes(kind)){showDialog(kind,id);return;}
  if(!source)return;
  if(kind==='step')ui.cursor=Number(id);
  else if(kind==='latest')ui.cursor=null;
  else if(kind==='prev'||kind==='next')ui.cursor=Math.max(0,Math.min((ui.cursor??maxFrame(source))+(kind==='prev'?-1:1),maxFrame(source)));
  redraw();
});
document.addEventListener('keydown',event=>{
  if(event.key==='Escape'&&!dialog.open){
    hideTip();const focused=ui.focusPlayer;ui.focusPlayer=null;ui.focusItem=null;redraw();
    if(focused)document.querySelector(`[data-action="focus-agent:${CSS.escape(focused)}"]`)?.focus();
  }
  const target=event.target.closest('[data-item]');
  if(target&&['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(event.key)){
    const items=[...document.querySelectorAll('[data-item]')],index=items.indexOf(target),delta=['ArrowLeft','ArrowUp'].includes(event.key)?-1:1;
    event.preventDefault();items[(index+delta+items.length)%items.length]?.focus();
  }
});
document.addEventListener('change',event=>{
  const key=event.target.dataset.control;
  if(['quality','sort'].includes(key)){ui[key]=event.target.value;if(key==='sort')live.refresh({results:true});else{redraw();updateDialog();}}
});
document.addEventListener('input',event=>{
  if(event.target.dataset.control==='search'){ui.search=event.target.value;updateDialog();}
});
$('language-button').addEventListener('click',()=>{setLocale(getLocale()==='en'?'zh-CN':'en');hideTip();redraw();updateDialog();});
$('rules-button').addEventListener('click',()=>showDialog('rules'));
function hideTip(){$('item-tooltip').hidden=true;}
function showTip(target){
  if(!source||ui.page!=='room'||$('content').inert)return hideTip();
  const item=knownItems(observation(source,ui.cursor)).find(item=>item.id===target.dataset.item);if(!item)return hideTip();
  const tip=$('item-tooltip'),box=target.getBoundingClientRect();tip.innerHTML=itemTip(item);tip.hidden=false;
  tip.style.left=Math.max(8,Math.min(innerWidth-tip.offsetWidth-8,box.right+9))+'px';
  tip.style.top=Math.max(8,Math.min(innerHeight-tip.offsetHeight-8,box.top))+'px';
}
$('content').addEventListener('pointerover',event=>{if(matchMedia('(hover:hover)').matches){const target=event.target.closest('[data-item]');if(target)showTip(target);}});
$('content').addEventListener('pointerout',event=>{if(event.target.closest('[data-item]')&&!event.target.closest('[data-item]').contains(event.relatedTarget))hideTip();});
$('content').addEventListener('focusin',event=>{const target=event.target.closest('[data-item]');if(target)showTip(target);});
$('content').addEventListener('focusout',hideTip);
addEventListener('resize',hideTip);

async function readJSON(path,signal) {
  const response=await fetch(path,{cache:'no-store',signal});
  if(!response.ok)throw new Error(String(response.status));
  return response.json();
}
function setObserver(value) {
  if(ui.observer===value)return;
  ui.observer=value;
  if(source)source={...source,decisions:[]};
  redraw();live.changeObserver();
}
const live=observe({getRoom:()=>ui.room,getObserver:()=>ui.observer,getSort:()=>ui.sort,read:readJSON,
  onObserverExpired(){ui.manage=false;setObserver(false);},
  onData(data){
    if(data.state){source=normalizeView(data.state);hideTip();}
    if(source&&!knownItems(observation(source,ui.cursor)).some(item=>item.id===ui.focusItem))ui.focusItem=null;
    if(data.rooms){
      const previous=rooms.find(room=>room.room_id===ui.room);
      rooms=data.rooms.rooms;
      const selected=rooms.find(room=>room.room_id===ui.room);
      if(ui.roomGroup===null)ui.roomGroup=selected?roomGroup(selected):rooms.some(room=>roomGroup(room)==='active')?'active':'completed';
      else if(selected&&previous&&roomGroup(selected)!==roomGroup(previous)&&ui.roomGroup===roomGroup(previous))ui.roomGroup=roomGroup(selected);
    }
    if(data.results){
      results=data.results;
      if(ui.agentProfile)ui.agentProfile=results.agents.find(agent=>agent.id===ui.agentProfile.id)||ui.agentProfile;
      updateDialog();
    }
    redraw();updateDialog();
  },
  onConnection(status){connection=status;redraw();},
  onResultsError(failed){resultsFailed=failed;}
});
let sessionUpdate;
function refreshSession(){
  if(!sessionUpdate)sessionUpdate=restoreSession().then(session=>{ui.manage=session.manage_rooms;setObserver(session.authenticated);redraw();}).finally(()=>{sessionUpdate=null;});
}
refreshSession();
addEventListener('focus',refreshSession);
addEventListener('pagehide',()=>live.close());
addEventListener('pageshow',event=>{if(event.persisted)location.reload();});
async function post(path,body){
  const response=await fetch(path,{method:'POST',headers:{'Content-Type':'application/json'},body:body?JSON.stringify(body):undefined,signal:AbortSignal.timeout(5000)});
  if(!response.ok)throw new Error(String(response.status));
  return response.json();
}
async function manageRoom(action){
  if(managing)return;managing=true;
  const room=ui.room;
  try{
    if(action==='invite-room'){
      const invitation=await post('/api/invite?room='+encodeURIComponent(room));
      showDialog('invite-room',new URL(invitation.invite_path,location.origin).href);
    }else await post('/api/start?room='+encodeURIComponent(room));
  }catch{showDialog('room-error');}
  finally{managing=false;live.refresh({state:true,rooms:true});}
}
dialog.addEventListener('submit',async event=>{
  if(event.target.id!=='create-room-form')return;
  event.preventDefault();if(managing)return;managing=true;
  const submit=event.target.querySelector('[type=submit]');submit.disabled=true;
  try{
    const data=new FormData(event.target),room=await post('/api/create',{name:data.get('name').trim(),players:Number(data.get('players'))});
    dialog.close();chooseRoom(room.room_id);live.refresh({state:true,rooms:true});
  }catch{$('create-room-error').hidden=false;}
  finally{managing=false;submit.disabled=false;}
});
redraw();


function fitCanvas(){
  document.querySelectorAll('[data-seat-y]').forEach(seat=>seat.style.setProperty('--seat-y',seat.dataset.seatY+'%'));
  document.querySelectorAll('.fitted-grid').forEach(grid=>{
    grid.style.setProperty('--columns',grid.dataset.columns);grid.style.setProperty('--rows',grid.dataset.rows);
    grid.querySelectorAll('[data-item]').forEach(item=>{
      for(const [property,key] of [['left','x'],['top','y'],['width','width'],['height','height']])item.style[property]=item.dataset[key]+'%';
    });
  });
}
