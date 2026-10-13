import {t} from './i18n.js';
import {esc} from './html.js';

// Cosmetic state stays local to the current room, separate from its observation.
let scene={room:null,lit:true,open:false,taps:0};
export function activateScene(kind){
  if(kind==='lamp')scene.lit=!scene.lit;
  else if(kind==='parcel')scene.open=!scene.open;
  else if(kind==='gavel')scene.taps=(scene.taps+1)%4;
  else return false;
  return true;
}
export function roomScene(room){
  if(scene.room!==room)scene={room,lit:true,open:false,taps:0};
  const control=(kind,label,art,pressed)=>`<button class="scene-prop scene-${kind}" data-action="scene:${kind}" aria-label="${esc(t(label))}" ${pressed===undefined?'':`aria-pressed="${pressed}"`}>${art}</button>`;
  return `<div class="room-scene ${scene.lit?'is-lit':''} ${scene.open?'parcel-open':''} ${scene.taps===3?'lucky-knock':''}" data-region="room-scene"><div class="warehouse-wall" aria-hidden="true"></div><div class="warehouse-floor" aria-hidden="true"></div>${control('lamp','sceneLamp',`<svg viewBox="0 0 100 140" aria-hidden="true"><defs><linearGradient id="warehouse-lamp-glow" x1="0" y1="84" x2="0" y2="430" gradientUnits="userSpaceOnUse"><stop stop-color="var(--scene-light)" stop-opacity=".6"/><stop offset=".55" stop-color="var(--scene-light)" stop-opacity=".18"/><stop offset="1" stop-color="var(--scene-light)" stop-opacity="0"/></linearGradient><filter id="warehouse-lamp-soften" x="-30%" y="-10%" width="160%" height="120%"><feGaussianBlur stdDeviation="5"/></filter></defs><g class="lamp-beam" filter="url(#warehouse-lamp-soften)"><path d="M40 84h20l100 346H-60z" fill="url(#warehouse-lamp-glow)"/></g><path class="lamp-cord" d="M50 0v49"/><g class="scene-shade"><path d="M39 49h22l18 23v6H21v-6z"/><path class="lamp-bulb" d="M38 78a12 12 0 0 0 24 0"/><path class="prop-detail" d="M32 70h36"/></g></svg>`,scene.lit)}${control('gavel','sceneGavel',`<svg viewBox="0 0 110 100" aria-hidden="true"><ellipse class="prop-shadow" cx="55" cy="88" rx="39" ry="6"/><path class="gavel-base" d="M23 75q32-12 64 0v9q-32 12-64 0z"/>${[0,1,2].map(i=>`<circle cx="${44+i*11}" cy="83" r="2" fill="var(--scene-ink)" opacity="${scene.taps>i?.7:.15}"/>`).join('')}<g class="scene-mallet"><path class="gavel-handle" d="m50 40 34 23q7 7 1 12l-4 1-38-27z"/><rect class="gavel-head" x="22" y="22" width="42" height="26" rx="7" transform="rotate(-35 43 35)"/><path class="prop-detail" d="m35 25 14 20"/></g><g class="knock-stars"><path d="m20 41 2-7 3 6 7 2-7 3-2 7-3-7-7-2zM79 20l2-6 3 5 6 2-6 3-2 6-3-6-6-2z"/></g></svg>`)}${control('parcel','sceneParcel',`<svg viewBox="1 0 110 110" aria-hidden="true">
  <ellipse class="prop-shadow" cx="56" cy="99" rx="39" ry="6"/>
  <path class="parcel-interior" d="m22 67 34-12 34 12-34 12z"/>
  <g class="scene-lid">
    <path class="parcel-top" d="m20 62 36-12 36 12-36 12z"/>
    <path class="parcel-edge" d="M20 62v8l36 12 36-12v-8L56 74z"/>
    <path class="parcel-tape" d="m45 54 34 12-11 4-34-12z"/>
  </g>
  <g class="parcel-visitor">
    <path class="visitor-antenna" d="M56 48V35"/><circle class="visitor-light" cx="56" cy="33" r="4"/>
    <rect class="visitor-body" x="32" y="42" width="48" height="37" rx="16"/>
    <path class="visitor-eyes" d="M44 56v5m24-5v5"/><path class="visitor-smile" d="M51 65q5 5 10 0"/>
  </g>
  <path class="parcel-box" d="m22 67 34 12 34-12v27l-34 9-34-10z"/>
  <path class="parcel-tape" d="m51 77 5 2 6-2v22l-6 2-5-2z"/>
</svg>`,scene.open)}</div>`;
}
export function sceneFeedback(button,kind,pointer){
  if(kind!=='gavel'||!pointer||matchMedia('(prefers-reduced-motion:reduce)').matches)return;
  const mallet=button.querySelector('.scene-mallet');
  if(!mallet)return;
  mallet.getAnimations().forEach(animation=>animation.cancel());
  mallet.animate([{transform:'rotate(0deg)'},{transform:'rotate(-24deg)',offset:.35},{transform:'rotate(12deg)',offset:.6},{transform:'rotate(0deg)'}],{duration:260,easing:'cubic-bezier(.23,1,.32,1)'});
}
