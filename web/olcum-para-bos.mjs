/** Boş ve yükleniyor durumları — üçlünün kalan ikisi. */
import { chromium } from 'playwright';
const KOK='http://localhost:3000';
const M={email:'yuk-1788931064-1@yuk.test',sifre:'yuk-testi-parolasi-uzun'};
const b=await chromium.launch(), c=await b.newContext({viewport:{width:1280,height:900}}), p=await c.newPage();
await p.goto(`${KOK}/giris`,{waitUntil:'networkidle'});
await p.fill('input[type="email"]',M.email); await p.fill('input[type="password"]',M.sifre);
await p.click('button[type="submit"]'); await p.waitForURL(/\/panel/,{timeout:20000});
// BOŞ
await p.route('**/api/v1/wallet/entries?**',(r)=>r.fulfill({status:200,contentType:'application/json',body:JSON.stringify({items:[],total:0,limit:25,offset:0})}));
await p.goto(`${KOK}/panel/cuzdan`,{waitUntil:'networkidle'}); await p.waitForTimeout(900);
const bos=await p.evaluate(()=>({
  metin:document.querySelector('main')?.innerText.split('\n').filter(Boolean).slice(-6),
  sayfalamaVar:!!document.querySelector('main nav[aria-label="Sayfalama"]'),
  sayacVar:!!document.querySelector('main .rounded-lg.border'),
}));
await p.screenshot({path:'/tmp/panel-para/bos-cuzdan-1280.png'});
console.log('BOŞ (cüzdan):',JSON.stringify(bos,null,1));
// YÜKLENİYOR (yanıtı geciktir)
await p.unrouteAll({behavior:'ignoreErrors'});
await p.route('**/api/v1/wallet/entries?**',async(r)=>{await new Promise(z=>setTimeout(z,9000));return r.continue();});
await p.goto(`${KOK}/panel/cuzdan`,{waitUntil:'domcontentloaded'}); await p.waitForTimeout(2500);
const yuk=await p.evaluate(()=>({
  iskeletSayisi:document.querySelectorAll('main .animate-pulse').length,
  spinnerVar:!!document.querySelector('main svg.animate-spin'),
  tabloIskeleti:!!document.querySelector('main table caption'),
  caption:document.querySelector('main table caption')?.textContent,
}));
await p.screenshot({path:'/tmp/panel-para/yukleniyor-cuzdan-1280.png'});
console.log('YÜKLENİYOR (cüzdan):',JSON.stringify(yuk));
await b.close();
