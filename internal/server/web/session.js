// A one-use host launch link establishes HttpOnly observer and room-manager cookies.
// Remove the fragment before awaiting network work; retain no credential.
export async function restoreSession({location=globalThis.location,history=globalThis.history,fetch=globalThis.fetch}={}) {
  const url=new URL(location.href);
  const code=new URLSearchParams(url.hash.slice(1)).get('launch');
  if(code!==null){url.hash='';history.replaceState(null,'',url);}
  try {
    if(code){
      await fetch('/api/observer/bootstrap',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({code}),cache:'no-store',signal:AbortSignal.timeout(5000)});
    }
    const response=await fetch('/api/observer/session',{cache:'no-store',signal:AbortSignal.timeout(5000)});
    if(response.ok){const session=await response.json();return {authenticated:session.authenticated===true,manage_rooms:session.manage_rooms===true};}
  }catch{}
  return {authenticated:false,manage_rooms:false};
}
