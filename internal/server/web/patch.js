// Preserve unchanged panels, focused controls, scroll containers and open details.
const key=node=>node.nodeType===1 ? ['data-region','data-control','data-action','data-item','id'].map(name=>node.hasAttribute(name)?name+':'+node.getAttribute(name):'').find(Boolean)||'' : '';
const compatible=(a,b)=>a.nodeType===b.nodeType && a.nodeName===b.nodeName && key(a)===key(b);
function patch(node,next) {
  if(node.isEqualNode(next))return;
  if(node.nodeType!==1){node.nodeValue=next.nodeValue;return;}
  for(const attr of [...node.attributes])if(!next.hasAttribute(attr.name) && !(node.localName==='details' && attr.name==='open'))node.removeAttribute(attr.name);
  for(const attr of next.attributes)if(node.getAttribute(attr.name)!==attr.value)node.setAttribute(attr.name,attr.value);
  reconcile(node,next);
  if(['input','select'].includes(node.localName) && node.value!==next.value)node.value=next.value;
}
function reconcile(parent,next) {
  let cursor=parent.firstChild;
  for(const desired of [...next.childNodes]){
    let current=cursor;
    if(!current || !compatible(current,desired)){
      current=key(desired)?[...parent.childNodes].find(node=>compatible(node,desired)):null;
      if(current)parent.insertBefore(current,cursor);
      else{current=desired.cloneNode(true);parent.insertBefore(current,cursor);}
    }
    patch(current,desired);cursor=current.nextSibling;
  }
  while(cursor){const removed=cursor;cursor=cursor.nextSibling;removed.remove();}
}
export function patchContent(element,html) {
  const template=element.ownerDocument.createElement('template');template.innerHTML=html;
  reconcile(element,template.content);
}
