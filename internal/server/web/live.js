// One global public or observer stream. Reads run only on invalidation or recovery.
export function observe({getRoom,getObserver=()=>false,getSort=()=>'profit',onObserverExpired=()=>{},onData,onConnection,onResultsError,read,EventSourceType=globalThis.EventSource}) {
  let stream,online=false,stopped=false,busy=false,epoch=0,controller,timer;
  let dirty={state:false,rooms:false,results:false};
  const schedule=(delay=0)=>{clearTimeout(timer);if(!stopped)timer=setTimeout(sync,delay);};
  function refresh(flags={state:true,rooms:true,results:true}) {
    for(const key of Object.keys(dirty))dirty[key] ||= !!flags[key];
    if(!busy && Object.values(dirty).some(Boolean))schedule();
  }
  async function sync() {
    if(stopped || busy)return;
    busy=true;
    const requested=dirty;dirty={state:false,rooms:false,results:false};
    const room=getRoom(),revision=epoch;
    if(!room)requested.state=false;
    controller=new AbortController();
    const signal=AbortSignal.any([controller.signal,AbortSignal.timeout(5000)]);
    const paths={state:(getObserver()?'/api/observer/replay':'/api/replay')+'?room='+encodeURIComponent(room),rooms:'/api/rooms',results:'/api/stats?sort='+encodeURIComponent(getSort())};
    const keys=Object.keys(requested).filter(key=>requested[key]);
    const replies=await Promise.allSettled(keys.map(key=>read(paths[key],signal)));
    if(stopped)return;
    if(revision===epoch && getObserver() && replies.some((reply,index)=>keys[index]==='state' && reply.status==='rejected' && reply.reason.message==='401'))onObserverExpired();
    let failed=false;
    if(revision===epoch){
      const data={};let stateOK=true;
      replies.forEach((reply,index)=>{
        const key=keys[index];
        if(reply.status==='fulfilled')data[key]=reply.value;
        else if(key==='state'&&reply.reason.message==='404'){data.roomMissing=room;dirty.rooms=true;}
        else if(key==='state'&&reply.reason.message==='503'){data.roomUnavailable=room;dirty.rooms=true;}
        else{dirty[key]=true;failed=true;if(key==='state')stateOK=false;}
        if(key==='results')onResultsError(reply.status!=='fulfilled');
      });
      onData(data);
      if(requested.state || !room)onConnection(online && stateOK?'connected':'disconnected');
    }else{
      dirty.rooms ||= requested.rooms;
      dirty.results ||= requested.results;
    }
    busy=false;
    if(Object.values(dirty).some(Boolean))schedule(failed?2000:0);
  }
  function connect() {
    if(!EventSourceType){
      // Older browsers keep the same observer UI with a conservative fallback.
      online=true;refresh();
      stream={close(){},fallback:setInterval(()=>refresh(),5000)};
      return;
    }
    const current=new EventSourceType(getObserver()?'/api/observer/events':'/api/events');stream=current;
    stream.addEventListener('ready',()=>{if(stopped || stream!==current)return;online=true;onConnection('connecting');refresh();});
    for(const name of ['room_created','room_archived','room_restored','room_deleted','state_changed','observer_changed','started','round_revealed','settled'])stream.addEventListener(name,event=>{
      if(stopped || stream!==current)return;
      try{
        const change=JSON.parse(event.data);
        refresh({state:change.room_id===getRoom(),rooms:change.rooms,results:change.results});
      }catch{refresh();}
    });
    stream.addEventListener('error',()=>{if(stopped || stream!==current)return;online=false;onConnection('disconnected');if(getObserver())refresh({state:true});});
  }
  connect();
  return {
    refresh,
    changeRoom(){epoch++;controller?.abort();onConnection('connecting');refresh({state:true});},
    changeObserver(){epoch++;controller?.abort();clearInterval(stream?.fallback);stream?.close();online=false;connect();onConnection('connecting');refresh();},
    close(){stopped=true;controller?.abort();clearTimeout(timer);clearInterval(stream?.fallback);stream?.close();}
  };
}
