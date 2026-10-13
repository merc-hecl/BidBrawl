export const esc=value=>String(value).replace(/[&<>"']/g,char=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
export const icon=name=>`<svg class="icon" aria-hidden="true"><use href="/icons.svg#${name}"></use></svg>`;
export const button=(text,action,cls='',extra='')=>`<button class="${cls}" data-action="${esc(action)}" ${extra}>${text}</button>`;
