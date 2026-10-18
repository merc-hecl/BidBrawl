import {t} from './i18n.js';
import {avatarGaze} from './avatars.js';

const root=document.documentElement,app=document.querySelector('.app'),workspace=document.querySelector('.workspace');
const reduced=matchMedia('(prefers-reduced-motion:reduce)'),fine=matchMedia('(hover:hover) and (pointer:fine)');
let expanded=false;
export function updateAppearance(){
  const dark=root.dataset.theme==='dark';
  app.classList.toggle('sidebar-open',expanded);app.classList.toggle('sidebar-collapsed',!expanded);
  workspace.inert=expanded;document.querySelector('.site-header').inert=expanded;
  const sidebar=document.getElementById('sidebar');sidebar.inert=!expanded;sidebar.setAttribute('aria-hidden',String(!expanded));
  const toggle=document.getElementById('sidebar-toggle');
  toggle.setAttribute('aria-expanded',String(expanded));toggle.setAttribute('aria-label',t(expanded?'collapseSidebar':'expandSidebar'));
  document.getElementById('theme-button').setAttribute('aria-pressed',String(dark));
  document.getElementById('theme-button').setAttribute('aria-label',t(dark?'switchLight':'switchDark'));
  document.getElementById('language-button').textContent=root.lang==='en'?'中文':'EN';
  document.querySelector('meta[name=theme-color]').content=dark?'#24251f':'#f3efe4';
}
export function setSidebar(value){expanded=value;updateAppearance();if(value)document.getElementById('sidebar-toggle').focus();}
export function closeMobileSidebar(){setSidebar(false);}
document.getElementById('sidebar-toggle').onclick=()=>setSidebar(!expanded);
document.querySelector('.sidebar-scrim').onclick=()=>setSidebar(false);
export function toggleTheme(){
  root.dataset.theme=root.dataset.theme==='dark'?'light':'dark';try{localStorage.setItem('bidbrawl.theme',root.dataset.theme);}catch{}updateAppearance();
};
document.getElementById('theme-button').onclick=toggleTheme;
document.addEventListener('keydown',event=>{document.body.classList.add('using-keyboard');if(event.key==='Escape'&&expanded){setSidebar(false);document.querySelector('[data-action="room-info"]')?.focus();}});
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
