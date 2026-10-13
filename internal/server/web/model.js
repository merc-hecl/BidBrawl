// The observer endpoint supplies decisions; player self/credentials are discarded.
export function normalizeView(raw) {
  return {room_id:raw.room_id, room_name:raw.room_name, version:raw.version, phase:raw.phase, round:raw.round,
    rules:raw.rules, players:raw.players || [], public_hints:raw.public_hints || [], history:raw.history || [],
    decisions:raw.decisions || [], settlement:raw.settlement,can_manage:raw.can_manage===true,frames:(raw.frames||[]).map(normalizeView)};
}
export function observation(source,cursor) {
  if(cursor===null || source.phase==='lobby')return source;
  return source.frames[Math.min(cursor,source.frames.length-1)];
}
export function knownItems(view) {
  if (view.settlement) return view.settlement.items;
  const known = new Map();
  for (const hint of view.public_hints) {
    const f=hint.data.fact;
    if (!f?.item_id)continue;
    if(f.type==='random_shape')known.set(f.item_id,{...known.get(f.item_id),id:f.item_id,top_left:f.top_left,bottom_right:f.bottom_right});
    if(f.type==='random_quality')known.set(f.item_id,{...known.get(f.item_id),id:f.item_id,quality:f.quality});
    if(f.type==='random_quality_price')known.set(f.item_id,{...known.get(f.item_id),id:f.item_id,quality:f.quality,price:f.price});
  }
  return [...known.values()].filter(item=>item.top_left&&item.bottom_right);
}
