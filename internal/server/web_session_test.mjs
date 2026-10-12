import test from 'node:test';
import assert from 'node:assert/strict';
import {restoreSession} from './web/session.js';

test('launch fragment is removed before bootstrap and never placed in requests URLs',async()=>{
  let clean=false;const calls=[];
  const authenticated=await restoreSession({location:{href:'http://localhost/?room=r&lang=en#launch=one-use'},history:{replaceState(_a,_b,url){clean=true;assert.equal(url.hash,'');assert.equal(url.search,'?room=r&lang=en');}},fetch:async(path,options)=>{
    assert.equal(clean,true);calls.push(path);
    if(path.endsWith('bootstrap'))assert.deepEqual(JSON.parse(options.body),{code:'one-use'});
    return {ok:true,json:async()=>({authenticated:true})};
  }});
  assert.deepEqual(authenticated,{authenticated:true,manage_rooms:false});
  assert.deepEqual(calls,['/api/observer/bootstrap','/api/observer/session']);
});

test('reload restores cookie without generating or requesting a launch code',async()=>{
  const calls=[];
  assert.deepEqual(await restoreSession({location:{href:'http://localhost/?room=r'},history:{replaceState(){assert.fail('no fragment to remove');}},fetch:async path=>{calls.push(path);return {ok:true,json:async()=>({authenticated:true,manage_rooms:true})};}}),{authenticated:true,manage_rooms:true});
  assert.deepEqual(calls,['/api/observer/session']);
});

test('invalid launch and expired sessions grant no private view; an existing cookie remains valid',async()=>{
  for(const authenticated of [false,true])assert.deepEqual(await restoreSession({location:{href:'http://localhost/#launch=expired'},history:{replaceState(){}},fetch:async path=>({ok:!path.endsWith('bootstrap'),json:async()=>({authenticated})})}),{authenticated,manage_rooms:false});
  assert.deepEqual(await restoreSession({location:{href:'http://localhost/'},history:{},fetch:async()=>{throw new Error('offline');}}),{authenticated:false,manage_rooms:false});
});
