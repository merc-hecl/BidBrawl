import {initialLocale,setLocale,getLocale,t,translateDOM} from './i18n.js';
import {normalizeView} from './model.js';
import {esc,renderContent,roomDirectory,roomButtons,maxFrame,dialogContent} from './render.js';
import {observe} from './live.js';
import {patchContent} from './patch.js';
import {restoreSession} from './session.js';

const $=id=>document.getElementById(id);
const text=(id,value)=>{const node=$(id),next=String(value);if(node.textContent!==next)node.textContent=next;};
const initialRoom=new URLSearchParams(location.search).get('room')||'';
const ui={room:initialRoom,page:initialRoom?'watch':'rooms',cursor:null,sort:'profit',quality:'all',search:'',observer:false,manage:false};
let source=null,rooms=[],results={games:0,profit:0,paid:0,rate:null,agents:[]},connection='connecting',resultsFailed=false;
let lastContent='',lastRooms='',lastOptions='';
let managing=false,activeDialog=null;
const dialog=document.querySelector('dialog');
setLocale(initialLocale(),false);

function redraw() {
  translateDOM();
  $('language-select').value=getLocale();
  text('page-name',t(ui.page));
  $('page-icon').className='icon icon-'+ui.page;
  document.querySelectorAll('[data-page]').forEach(button=>{
    const selected=button.dataset.page===ui.page;
    button.classList.toggle('selected',selected);
    if(selected)button.setAttribute('aria-current','page');else button.removeAttribute('aria-current');
  });
  text('connection',t(connection));
  $('connection').classList.toggle('offline',connection==='disconnected');
  $('error').hidden=connection!=='disconnected' && !(ui.page==='stats' && resultsFailed);
  text('error',t(connection==='disconnected'?'loadError':'resultsError'));
  text('room-count',rooms.length);
  const options=rooms.map(room=>`<option value="${esc(room.room_id)}">${esc(room.name)}</option>`).join('');
  if(lastOptions!==options){$('room-select').innerHTML=options;lastOptions=options;}
  $('room-select').value=ui.room;
  $('room-select').parentElement.hidden=ui.page==='stats'||ui.page==='rooms';
  $('rules-button').hidden=!source||ui.page==='stats'||ui.page==='rooms';
  const side=roomButtons(rooms,ui.room);
  if(lastRooms!==side){$('rooms').innerHTML=side;lastRooms=side;}
  const html=ui.page==='rooms'?roomDirectory(rooms,ui):renderContent(source,results,ui);
  if(lastContent!==html){
    patchContent($('content'),html);lastContent=html;
  }
  $('content').setAttribute('aria-busy',connection==='connecting'?'true':'false');
}
function updateDialog() {
  if(!activeDialog)return;
  const content=dialogContent(activeDialog.kind,activeDialog.id,source,results,ui);
  if(!content){dialog.close();return;}
  dialog.innerHTML=`<div class="dialog-head"><h2 id="dialog-title">${esc(content.title)}</h2><button class="dialog-close" aria-label="${esc(t('close'))}"><span class="icon icon-close" aria-hidden="true"></span></button></div>${content.content}`;
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
  ui.page='watch';
  if(id===ui.room){redraw();return;}
  ui.room=id;ui.cursor=ui.page==='replay'?0:null;ui.quality='all';ui.search='';
  source=null;connection='connecting';
  const url=new URL(location.href);url.searchParams.set('room',id);history.replaceState(null,'',url);
  redraw();live.changeRoom();
}
document.addEventListener('click',event=>{
  const page=event.target.closest('[data-page]')?.dataset.page;
  if(page){ui.page=!ui.room&&['watch','replay'].includes(page)?'rooms':page;ui.cursor=page==='replay'?(source?.history.length?1:0):null;redraw();return;}
  const item=event.target.closest('[data-item]')?.dataset.item;
  if(item){showDialog('item',item);return;}
  const action=event.target.closest('[data-action]')?.dataset.action;
  if(!action)return;
  const separator=action.indexOf(':'),kind=separator<0?action:action.slice(0,separator),id=separator<0?'':action.slice(separator+1);
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
  if(!['Enter',' '].includes(event.key))return;
  const item=event.target.closest('[data-item]')?.dataset.item;
  if(item){event.preventDefault();showDialog('item',item);}
});
$('content').addEventListener('change',event=>{
  const key=event.target.dataset.control;
  if(['quality','sort'].includes(key)){ui[key]=event.target.value;if(key==='sort')live.refresh({results:true});else redraw();}
});
$('content').addEventListener('input',event=>{
  if(event.target.dataset.control==='search'){ui.search=event.target.value;redraw();}
});
$('room-select').addEventListener('change',event=>chooseRoom(event.target.value));
$('language-select').addEventListener('change',event=>{setLocale(event.target.value);redraw();updateDialog();});
$('rules-button').addEventListener('click',()=>showDialog('rules'));

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
    if(data.state){
      const firstLoad=!source;source=normalizeView(data.state);
      if(firstLoad && ui.page==='replay')ui.cursor=source.history.length?1:0;
    }
    if(data.rooms)rooms=data.rooms.rooms;
    if(data.results){
      results=data.results;
      if(ui.agentProfile)ui.agentProfile=results.agents.find(agent=>agent.id===ui.agentProfile.id)||ui.agentProfile;
      updateDialog();
    }
    redraw();
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
