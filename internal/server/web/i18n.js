import zh from './locales/zh-CN.js';
import en from './locales/en.js';

export const catalogs = {'zh-CN': zh, en};
export const supportedLocales = Object.keys(catalogs);
let locale = 'zh-CN';
const normalize = value => value?.toLowerCase().startsWith('zh') ? 'zh-CN' : value?.toLowerCase().startsWith('en') ? 'en' : null;

export function initialLocale() {
  let saved;
  try { saved = localStorage.getItem('bidbrawl.locale'); } catch { /* Storage can be disabled. */ }
  return normalize(new URLSearchParams(location.search).get('lang')) || normalize(saved) ||
    navigator.languages?.map(normalize).find(Boolean) || 'zh-CN';
}
export function setLocale(value, persist = true) {
  locale = normalize(value) || 'zh-CN';
  if (typeof document !== 'undefined') {
    document.documentElement.lang = locale;
    document.title = t('title');
    if (persist) {
      try { localStorage.setItem('bidbrawl.locale', locale); } catch { /* Optional preference. */ }
      const url = new URL(location.href);
      if (url.searchParams.has('lang')) { url.searchParams.set('lang', locale); history.replaceState(null, '', url); }
    }
  }
  return locale;
}
export const getLocale = () => locale;
export function t(key, values = {}) {
  const message = catalogs[locale][key] ?? zh[key];
  if (message === undefined) throw new Error(`Missing translation: ${key}`);
  const template = typeof message === 'string' ? message :
    message[new Intl.PluralRules(locale).select(Number(values.count))] ?? message.other;
  return template.replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? `{${name}}`));
}
export const money = (value, maximumFractionDigits = 0) => new Intl.NumberFormat(locale, {maximumFractionDigits}).format(value);
export const signed = (value, maximumFractionDigits = 0) => (value >= 0 ? '+' : '−') + money(Math.abs(value), maximumFractionDigits);
export function localizeCard(card) {
  const nameKey = `card.${card.id}.name`, descriptionKey = `card.${card.id}.description`;
  // Only translate unchanged built-in text. Host-authored names and descriptions are data.
  if (card.name !== zh[nameKey] || card.description !== zh[descriptionKey]) return card;
  return {...card, name:t(nameKey), description:t(descriptionKey)};
}
export function localizeName(kind,definition) {
  if(!definition)return '';
  const key=`${kind}.${definition.id}.name`;
  return zh[key]===definition.name?t(key):definition.name;
}
export function localizeItemName(item,catalog=[]) {
  if(!item.name)return '';
  const template=item.template_id?catalog.find(entry=>entry.id===item.template_id&&entry.name===item.name):null;
  return item.translations?.[locale]||template?.translations?.[locale]||item.name;
}
export function translateDOM(root = document) {
  for (const [attribute, target] of [['data-i18n', null], ['data-i18n-aria', 'aria-label'], ['data-i18n-placeholder', 'placeholder']]) {
    root.querySelectorAll(`[${attribute}]`).forEach(node => {
      const text = t(node.getAttribute(attribute));
      if (target) {if(node.getAttribute(target)!==text)node.setAttribute(target,text);}
      else if(node.textContent!==text)node.textContent=text;
    });
  }
}
