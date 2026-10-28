import test from 'node:test';
import assert from 'node:assert/strict';
import {observe} from './web/live.js';

class Stream {
  static latest;
  handlers=new Map();
  constructor(url){assert.ok(['/api/events','/api/observer/events'].includes(url));this.url=url;Stream.latest=this;}
  addEventListener(name,fn){this.handlers.set(name,fn);}
  emit(name,data={}){this.handlers.get(name)({data:JSON.stringify(data)});}
  close(){this.closed=true;}
}
const tick=()=>new Promise(resolve=>setTimeout(resolve,20));

test('observer credential expiration switches to public stream without private state',async()=>{
  let observer=true,live;const updates=[],reads=[];
  live=observe({getRoom:()=> 'table',getObserver:()=>observer,EventSourceType:Stream,
    onObserverExpired(){observer=false;live.changeObserver();},
    read:async path=>{reads.push(path);if(path.startsWith('/api/observer/replay'))throw new Error('401');return path.includes('replay')?{phase:'bidding'}:{rooms:[]};},
    onData:data=>updates.push(data),onConnection:()=>{},onResultsError:()=>{}});
  try {
    const privateStream=Stream.latest;assert.equal(privateStream.url,'/api/observer/events');privateStream.emit('ready');await tick();
    assert.equal(privateStream.closed,true);assert.equal(Stream.latest.url,'/api/events');
    Stream.latest.emit('ready');await tick();
    assert.ok(reads.includes('/api/observer/replay?room=table')&&reads.includes('/api/replay?room=table'));
    assert.ok(updates.every(data=>!data.state?.decisions));
  }finally{live.close();}
});
