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

test('unavailable room returns to the directory without repeating replay reads',async()=>{
  let room='unavailable';const reads=[],updates=[];
  const live=observe({getRoom:()=>room,EventSourceType:Stream,
    read:async path=>{reads.push(path);if(path.includes('room=unavailable'))throw new Error('503');return path.includes('replay')?{room_id:room}:{rooms:[]};},
    onData:data=>{updates.push(data);if(data.roomUnavailable===room){room='';live.changeRoom();}},
    onConnection:()=>{},onResultsError:()=>{}});
  try{
    Stream.latest.emit('ready');await tick();await tick();
    assert.ok(updates.some(data=>data.roomUnavailable==='unavailable'));
    assert.equal(reads.filter(path=>path.includes('replay')).length,1);
    room='healthy';live.changeRoom();await tick();
    assert.equal(reads.filter(path=>path.includes('room=healthy')).length,1);
  }finally{live.close();}
});

test('room lifecycle events invalidate rooms and a deleted selection stops replay retries',async()=>{
  let room='table';
  const reads=[],updates=[];
  const live=observe({getRoom:()=>room,EventSourceType:Stream,
    read:async path=>{reads.push(path);if(path.includes('replay'))throw new Error('404');return {rooms:[]};},
    onData:data=>{updates.push(data);if(data.roomMissing===room){room='';live.changeRoom();}},
    onConnection:()=>{},onResultsError:()=>{}});
  try{
    Stream.latest.emit('room_deleted',{room_id:'table',rooms:true,results:false});await tick();await tick();
    assert.ok(updates.some(data=>data.roomMissing==='table'));
    assert.equal(reads.filter(path=>path.includes('replay')).length,1);
    for(const event of ['room_archived','room_restored']){
      const before=reads.length;
      Stream.latest.emit(event,{room_id:'other',rooms:true,results:false});await tick();
      assert.deepEqual(reads.slice(before),['/api/rooms']);
    }
    assert.ok(!reads.some(path=>path.includes('/stats')));
  }finally{live.close();}
});

test('live observer reads on events, stays idle, and fully syncs on reconnect',async()=>{
  const reads=[],updates=[],connections=[];
  const live=observe({getRoom:()=> 'table',EventSourceType:Stream,
    read:async path=>{reads.push(path);return path.includes('replay')?{phase:'bidding'}:{rooms:[]};},
    onData:data=>updates.push(data),onConnection:state=>connections.push(state),onResultsError:()=>{}});
  try{
    Stream.latest.emit('ready');await tick();
    assert.equal(reads.length,3);assert.equal(connections.at(-1),'connected');
    await new Promise(resolve=>setTimeout(resolve,900));assert.equal(reads.length,3);
    Stream.latest.emit('state_changed',{room_id:'other',rooms:false,results:false});await tick();
    assert.equal(reads.length,3);
    Stream.latest.emit('state_changed',{room_id:'table',rooms:false,results:false});await tick();
    assert.deepEqual(reads.slice(3),['/api/replay?room=table']);
    Stream.latest.emit('settled',{room_id:'other',rooms:true,results:true});await tick();
    assert.deepEqual(reads.slice(4),['/api/rooms','/api/stats?sort=profit']);
    Stream.latest.emit('error');assert.equal(connections.at(-1),'disconnected');
    Stream.latest.emit('ready');await tick();assert.equal(reads.length,9);
    assert.equal(connections.at(-1),'connected');
    assert.ok(updates.at(-1).state && updates.at(-1).rooms && updates.at(-1).results);
  }finally{live.close();}
  assert.equal(Stream.latest.closed,true);
});

test('room switch cancels stale state but retains pending metadata invalidations',async()=>{
  let room='old',first=true;
  const reads=[],updates=[];
  const live=observe({getRoom:()=>room,EventSourceType:Stream,
    read:(path,signal)=>{
      reads.push(path);
      if(first)return new Promise((resolve,reject)=>signal.addEventListener('abort',()=>reject(new Error('aborted'))));
      return Promise.resolve(path.includes('replay')?{room_id:room}:{rooms:[]});
    },onData:data=>updates.push(data),onConnection:()=>{},onResultsError:()=>{}});
  try{
    Stream.latest.emit('ready');await tick();
    room='new';first=false;live.changeRoom();await tick();
    assert.equal(updates.length,1);
    assert.equal(updates[0].state.room_id,'new');
    assert.ok(updates[0].rooms && updates[0].results);
    assert.deepEqual(reads.slice(3),['/api/replay?room=new','/api/rooms','/api/stats?sort=profit']);
  }finally{live.close();}
});

test('inflight events are applied after the current snapshot rather than dropped',async()=>{
  const reads=[],updates=[];
  const connections=[];
  let release;
  const gate=new Promise(resolve=>release=resolve);
  const live=observe({getRoom:()=> 'table',EventSourceType:Stream,
    read:async path=>{reads.push(path);if(reads.length<=3)await gate;return {rooms:[]};},
    onData:data=>updates.push(data),onConnection:state=>connections.push(state),onResultsError:()=>{}});
  try{
    Stream.latest.emit('ready');await tick();
    Stream.latest.emit('round_revealed',{room_id:'table',rooms:false,results:false});
    release();await tick();
    assert.deepEqual(reads.slice(3),['/api/replay?room=table']);
    assert.equal(updates.length,2);
    assert.ok(!connections.includes('disconnected'));
  }finally{live.close();}
});

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
