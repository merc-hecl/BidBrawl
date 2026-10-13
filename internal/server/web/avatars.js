export const avatars=[
  {path:'M50 10C75 8 93 27 91 53C89 80 71 94 46 91C20 88 9 68 11 44C13 23 27 12 50 10Z'},
  {path:'M49 9C57-1 69 4 69 16C89 19 99 36 91 53C100 72 84 92 64 88C50 100 29 92 24 81C5 77 4 53 16 42C9 23 29 8 49 9Z'},
  {path:'M32 12Q50 5 68 12L90 69Q99 90 76 92H24Q1 90 10 69Z'},
  {path:'M24 30C22 6 53 1 65 20C85 9 106 36 89 53C108 77 75 99 58 85C38 105 10 88 18 66C-4 54 4 31 24 30Z'},
  {path:'M40 9Q50 2 60 9L90 39Q100 50 90 61L60 91Q50 98 40 91L10 61Q0 50 10 39Z'},
  {path:'M56 8C13 3-6 48 17 78C34 101 74 99 91 72C62 79 37 50 56 8Z'},
  {path:'M25 12C41 9 63 10 77 14Q90 17 89 34L86 74Q85 88 70 89L27 87Q12 86 12 71L11 30Q10 15 25 12Z',shine:'M25 27Q34 20 45 22'},
];

export function avatarGaze(index,x,y){
  return index===5?{x:Math.max(-2,Math.min(1.2,x)),y:Math.max(-1.5,Math.min(2,y))}:{x,y};
}

export function avatar(index,{winner=false,large=false,context='seat'}={}){
  const a=avatars[index];
  if(!a)return '<span class="avatar-pending" aria-hidden="true"><svg class="icon"><use href="/icons.svg#users"></use></svg></span>';
  const clip='bot-clip-'+context+'-'+index,moon=index===5;
  return `<span class="bot-avatar bot-${index} ${winner?'bot-winner':''} ${large?'bot-large':''}" data-bot="${index}" aria-hidden="true"><svg viewBox="0 0 100 108"><defs><clipPath id="${clip}"><path d="${a.path}"/></clipPath></defs><ellipse class="bot-ground" cx="50" cy="100" rx="23" ry="3"/><g class="bot-body"><path class="bot-shape" d="${a.path}"/><path class="bot-shine" d="${a.shine||'M28 27Q35 18 45 19'}"/><g clip-path="url(#${clip})"><g class="bot-look"><g transform="translate(${moon?-12:0} ${moon?12:0})"><g class="bot-eyes"><rect class="bot-eye eye-left" x="${moon?30:36}" y="39" width="${moon?7:9}" height="${moon?17:20}" rx="4.5"/><rect class="bot-eye eye-right" x="${moon?45:56}" y="37" width="${moon?7:9}" height="${moon?17:20}" rx="4.5"/></g></g></g></g></g></svg>${winner?'<span class="bot-star">✦</span>':''}</span>`;
}
