import {t,localizeItemName} from './i18n.js';
import {esc} from './html.js';
import {itemArtSource,itemArtwork} from './item-art.js';

const collections=new WeakMap();
const layouts=[[[128,'sand'],[194,'paper'],[216,'green']],[[226,'green'],[150,'sand'],[164,'paper']],[[184,'paper'],[164,'sand'],[120,'paper']]];

export function waterfall(catalog=[]){
  if(!collections.has(catalog)){
    const pool=catalog.filter(item=>item.category!=='easter_egg'&&itemArtSource(item,catalog));
    const selected=[];
    while(pool.length&&selected.length<9)selected.push(pool.splice(Math.floor(Math.random()*pool.length),1)[0]);
    collections.set(catalog,selected);
  }
  const selected=collections.get(catalog);
  if(!selected.length)return '';
  return `<div class="waterfall" aria-hidden="true">${layouts.map((layout,i)=>{
    const cards=layout.map(([height,tone],j)=>{
      const item=selected[i*3+j];
      return item?`<div class="collection-card tone-${tone}" data-template="${esc(item.id)}" style="height:${height}px">${itemArtwork(item,catalog)}<span>${esc(localizeItemName(item,catalog))}</span></div>`:'';
    }).join('');
    return cards?`<div class="waterfall-column column-${i}">${[0,1].map(()=>`<div class="waterfall-group">${cards}</div>`).join('')}</div>`:'';
  }).join('')}<span class="collection-caption">${esc(t('collection'))}</span></div>`;
}
