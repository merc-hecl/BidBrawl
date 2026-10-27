// Private observations only survive while the human observer session is active.
export function normalizeView(raw,observer=false) {
  return {room_id:raw.room_id, room_name:raw.room_name, version:raw.version, phase:raw.phase, round:raw.round,round_deadline:raw.round_deadline,
    activities:observer?(raw.activities||[]):[],player_costs:observer?raw.player_costs:undefined,context:raw.context,intelligence:observer?raw.intelligence:undefined,rules:raw.rules, players:raw.players || [], public_hints:raw.public_hints || [], history:raw.history || [],
    decisions:observer?(raw.decisions || []):[], initial_inspections:observer?raw.initial_inspections:undefined,specialty_states:observer?raw.specialty_states:undefined,
    item_reveals:observer?raw.item_reveals:undefined,items:observer?raw.items:undefined,total_value:observer?raw.total_value:undefined,
    settlement:raw.settlement,can_manage:observer&&raw.can_manage===true,frames:(raw.frames||[]).map(frame=>normalizeView(frame,observer))};
}
export function observation(source,cursor) {
  if(cursor===null || source.phase==='lobby')return source;
  return source.frames[Math.min(cursor,source.frames.length-1)];
}
export function knownItems(view,playerID=null) {
  if (view.settlement) return view.settlement.items;
  if (!playerID && view.items) return view.items;
  const known = new Map();
  for(const item of view.initial_inspections?.[playerID] || [])known.set(item.id,{id:item.id,quality:item.quality,top_left:item.top_left,bottom_right:item.bottom_right});
  for(const reveal of view.item_reveals?.[playerID] || []) {
    const item={...known.get(reveal.item_id),id:reveal.item_id};
    for(const field of ['name','category','template_id','quality','price','top_left','bottom_right'])if(reveal[field]!==undefined)item[field]=reveal[field];
    known.set(item.id,item);
  }
  for (const hint of view.public_hints) {
    const f=hint.data.fact;
    if (!f?.item_id)continue;
    if(f.type==='random_shape')known.set(f.item_id,{...known.get(f.item_id),id:f.item_id,top_left:f.top_left,bottom_right:f.bottom_right});
    if(f.type==='random_quality')known.set(f.item_id,{...known.get(f.item_id),id:f.item_id,quality:f.quality});
  }
  return [...known.values()].filter(item=>item.top_left&&item.bottom_right);
}
