/** Hata durumu: HataDurumu requestId'yi gösteriyor mu? (eski kod göstermiyordu) */
import { chromium } from 'playwright';
const KOK='http://localhost:3000';
const M={email:'yuk-1788931064-1@yuk.test',sifre:'yuk-testi-parolasi-uzun'};
const HATA={error:{code:'INTERNAL',message:'Beklenmeyen bir hata oluştu.',requestId:'11111111-2222-4333-8444-555555555555'}};
const b=await chromium.launch(), c=await b.newContext({viewport:{width:1280,height:900}}), p=await c.newPage();
await p.goto(`${KOK}/giris`,{waitUntil:'networkidle'});
await p.fill('input[type="email"]',M.email); await p.fill('input[type="password"]',M.sifre);
await p.click('button[type="submit"]'); await p.waitForURL(/\/panel/,{timeout:20000});
const sonuc={};
for (const [ad,yol,desen] of [
  ['cuzdan','/panel/cuzdan','**/api/v1/wallet/entries?**'],
  ['bakiye-yukle-liste','/panel/bakiye-yukle','**/api/v1/wallet/deposits?**'],
  ['bakiye-yukle-yontem','/panel/bakiye-yukle','**/api/v1/wallet/deposit-methods'],
]) {
  await p.unrouteAll({ behavior: 'ignoreErrors' });
  await p.route(desen,(r)=>r.fulfill({status:500,contentType:'application/json',body:JSON.stringify(HATA)}));
  await p.goto(`${KOK}${yol}`,{waitUntil:'networkidle'});
  await p.waitForTimeout(14000);
  sonuc[ad]=await p.evaluate(()=>{
    const a=[...document.querySelectorAll('main [role="alert"]')].map(e=>e.textContent.trim().replace(/\s+/g,' '));
    const rid=[...document.querySelectorAll('main *')].some(e=>e.textContent?.includes('11111111-2222-4333-8444-555555555555'));
    const fs=[...document.querySelectorAll('main *')].filter(e=>!e.children.length&&e.textContent?.includes('11111111')).map(e=>getComputedStyle(e.parentElement).fontSize+' opak:'+getComputedStyle(e.parentElement).opacity);
    return {uyarilar:a, requestIdGorunuyor:rid, ridStil:fs};
  });
  await p.screenshot({path:`/tmp/panel-para/hata-${ad}.png`,fullPage:false});
}
console.log(JSON.stringify(sonuc,null,2));
await b.close();
