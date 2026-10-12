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
    const data = hint.data;
    if (data.item_id) known.set(data.item_id, {...known.get(data.item_id), id:data.item_id, ...data});
  }
  return [...known.values()].filter(item => item.top_left && item.bottom_right);
}
