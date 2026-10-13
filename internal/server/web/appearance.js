import {t} from './i18n.js';
import {icon} from './html.js';
import {avatarGaze} from './avatars.js';

const root=document.documentElement,app=document.querySelector('.app'),workspace=document.querySelector('.workspace');
const narrow=matchMedia('(max-width:900px)'),reduced=matchMedia('(prefers-reduced-motion:reduce)'),fine=matchMedia('(hover:hover) and (pointer:fine)');
let expanded=!narrow.matches;
try{expanded=localStorage.getItem('bidbrawl.sidebar')===null?expanded:localStorage.getItem('bidbrawl.sidebar')==='open';}catch{}
export function updateAppearance(){
  const dark=root.dataset.theme==='dark';
  app.classList.toggle('sidebar-open',expanded);app.classList.toggle('sidebar-collapsed',!expanded);
  workspace.inert=narrow.matches&&expanded;
  const toggle=document.getElementById('sidebar-toggle');
  toggle.setAttribute('aria-expanded',String(expanded));toggle.setAttribute('aria-label',t(expanded?'collapseSidebar':'expandSidebar'));
  document.getElementById('theme-button').innerHTML=icon(dark?'sun':'moon');
  document.getElementById('theme-button').setAttribute('aria-label',t(dark?'switchLight':'switchDark'));
}
export function setSidebar(value){expanded=value;try{localStorage.setItem('bidbrawl.sidebar',value?'open':'collapsed');}catch{}updateAppearance();}
export function closeMobileSidebar(){if(narrow.matches)setSidebar(false);}
document.getElementById('sidebar-toggle').onclick=()=>setSidebar(!expanded);
document.querySelector('.sidebar-scrim').onclick=()=>setSidebar(false);
document.getElementById('theme-button').onclick=()=>{
  root.dataset.theme=root.dataset.theme==='dark'?'light':'dark';try{localStorage.setItem('bidbrawl.theme',root.dataset.theme);}catch{}updateAppearance();
};
narrow.addEventListener('change',()=>setSidebar(!narrow.matches));
document.addEventListener('keydown',event=>{document.body.classList.add('using-keyboard');if(event.key==='Escape'&&narrow.matches&&expanded){setSidebar(false);document.getElementById('sidebar-toggle').focus();}});
document.addEventListener('pointerdown',()=>document.body.classList.remove('using-keyboard'));
document.addEventListener('visibilitychange',()=>document.body.classList.toggle('page-hidden',document.hidden));
let gazeFrame=0,pointer;
document.addEventListener('pointermove',event=>{
  if(!fine.matches||reduced.matches||document.hidden)return;
  pointer={x:event.clientX,y:event.clientY};
  if(gazeFrame)return;
  gazeFrame=requestAnimationFrame(()=>{
    gazeFrame=0;
    document.querySelectorAll('.bot-avatar').forEach(bot=>{
      const box=bot.getBoundingClientRect(),gaze=avatarGaze(Number(bot.dataset.bot),Math.max(-4,Math.min(4,(pointer.x-box.x-box.width/2)/80)),Math.max(-3,Math.min(3,(pointer.y-box.y-box.height/2)/100)));
      bot.querySelector('.bot-look').style.transform=`translate(${gaze.x}px,${gaze.y}px)`;
    });
  });
});
reduced.addEventListener('change',()=>{if(reduced.matches)document.querySelectorAll('.bot-look').forEach(look=>look.style.removeProperty('transform'));});
export function greet(button){const bot=button.querySelector('.bot-avatar');if(!bot)return;bot.classList.remove('is-greeting');requestAnimationFrame(()=>{bot.classList.add('is-greeting');setTimeout(()=>bot.classList.remove('is-greeting'),450);});}
