// 使用真正的管理介面及隔離 API；不安裝套件、不關閉 APP、不操作系統服務。
const fs=require('fs'),path=require('path'),assert=require('assert'),{chromium}=require('playwright');
const root=path.resolve(__dirname,'../internal/clientui/web'),origin='https://yourdesk-local-update.test';
const translations=JSON.parse(fs.readFileSync(path.join(root,'translations.json'),'utf8'));
(async()=>{const browser=await chromium.launch({headless:true});try{
 for(const platform of ['windows','darwin']){
  const page=await browser.newPage({viewport:{width:820,height:640}}),errors=[],writes=[];
  page.on('pageerror',error=>errors.push(error.message));
  const extension=platform==='windows'?'.zip':'.dmg';
  const state={info:{room:'YD-SMOKE',hostname:'Offline Smoke',platform,architecture:'arm64',version:'1.26.1015 build 1200'},library:{groups:[],sites:[]},preferences:{language:'zh-Hant',theme:'light',selectedGroup:'*'},running:{},sessions:{},updates:{localExtension:extension},hardwareDetection:{status:'complete'},audioCapabilities:[],prelogin:{supported:true}};
  let generation=0,mode='success',holdUpload=null,external=0;
  await page.route('**/*',async route=>{
   const request=route.request(),url=new URL(request.url());
   if(url.origin!==origin){external++;return route.abort();}
   if(url.pathname.startsWith('/api/')){
    const method=request.method();if(method!=='GET')writes.push([method,url.pathname]);
    if(url.pathname==='/api/updates/local'&&method==='POST'){
     const body=request.postDataJSON();assert.equal(body.name,'離線更新'+extension);assert.equal(body.size,7);
     state.updates.local={...body,id:String(++generation),phase:'selected',bytes:0};
     return route.fulfill({json:state.updates});
    }
    if(url.pathname==='/api/updates/local'&&method==='PUT'){
     assert.equal(request.postDataBuffer().toString(),'package','上傳必須使用原始檔案，不可當成本機路徑');
     const id=url.searchParams.get('id');assert.equal(id,state.updates.local.id);
     state.updates.local.phase='validating';
     if(mode==='slow')await new Promise(resolve=>{holdUpload=resolve;});
     if(state.updates.local?.id!==id)return route.fulfill({status:400,json:{error:'本地更新已取消或已過期，請重新選擇套件。'}}).catch(()=>{});
     if(mode==='invalid'){
      Object.assign(state.updates.local,{phase:'failed',error:'更新套件的版本資訊無效。'});
      return route.fulfill({status:400,json:{error:state.updates.local.error}});
     }
     // 比目前版本舊仍可準備完成，確認安裝前絕不能自動倒數。
     Object.assign(state.updates.local,{phase:'ready',version:'1.26.1003 build 2028',architecture:'arm64',requiresAuthorization:true});
     return route.fulfill({json:state.updates});
    }
    if(url.pathname==='/api/updates/local/cancel'){
     assert.equal(request.postDataJSON().id,state.updates.local.id);
     delete state.updates.local;
     return route.fulfill({json:state.updates});
    }
    if(url.pathname==='/api/updates/local/install'){
     assert.equal(state.updates.local.phase,'ready');assert.equal(request.postDataJSON().id,state.updates.local.id);
     state.updates.local.phase='installing';return route.fulfill({json:state.updates});
    }
    return route.fulfill({json:url.pathname==='/api/state'?state:url.pathname==='/api/updates/state'?state.updates:url.pathname==='/api/presence/local'?{room:'YD-SMOKE',online:null}:{}});
   }
   const file=path.resolve(root,url.pathname==='/'?'index.html':url.pathname.slice(1));
   if(!file.startsWith(root+path.sep)||!fs.existsSync(file))return route.abort();
   return route.fulfill({body:fs.readFileSync(file),contentType:({'.html':'text/html','.js':'text/javascript','.css':'text/css','.json':'application/json'})[path.extname(file)]||'application/octet-stream'});
  });
  await page.goto(origin+'/#smoke');
  await page.waitForFunction(()=>!!state);
  await page.locator('#open-settings').click();
  await page.locator('#settings-tab-about').click();
  assert.equal(await page.locator('#local-update-file').getAttribute('accept'),extension);
  assert.equal(await page.locator('#local-update').isVisible(),true);
  await page.locator('#force-update').check();
  for(const [language,index] of [['en',0],['ja',1],['ko',2],['zh-Hant',-1]]){
   await page.evaluate(language=>{i18n.apply(language);renderRelease(state.updates);},language);
   assert.equal(await page.locator('#local-update').textContent(),index<0?'本地更新':translations['本地更新'][index]);
   assert.equal(await page.locator('#settings-panel-about').evaluate(el=>el.scrollWidth<=el.clientWidth),true,'四語按鈕不可使設定區溢出');
  }
  await page.locator('#settings-tab-about').click();
  await page.mouse.move(0,0);
  fs.mkdirSync(path.resolve(__dirname,'../.local-run'),{recursive:true});
  await page.screenshot({path:path.resolve(__dirname,`../.local-run/local-update-about-${platform}.png`)});
  const choose=()=>page.locator('#local-update-file').setInputFiles({name:'離線更新'+extension,mimeType:'application/octet-stream',buffer:Buffer.from('package')});
  await choose();
  await page.waitForFunction(()=>document.querySelector('#local-update-install').disabled===false);
  assert.match(await page.locator('#local-update-version').textContent(),/1\.26\.1015.*→ 1\.26\.1003/);
  assert.equal(await page.locator('#local-update-authorization').isVisible(),true);
  assert.equal(await page.locator('#check-update').isDisabled(),true);
  assert.equal(await page.locator('#install-prerelease').isDisabled(),true);
  assert.equal(writes.filter(x=>x[1].endsWith('/install')).length,0,'套件驗證完成不可自動開始安裝');
  assert.equal(await page.evaluate(()=>state.updates.installAt || 0),0);
  await page.screenshot({path:path.resolve(__dirname,`../.local-run/local-update-ready-${platform}.png`)});
  await page.keyboard.press('Escape');
  await page.waitForFunction(()=>!document.querySelector('#local-update-dialog').open);
  assert.equal(state.updates.local,undefined,'Esc 必須取消伺服器工作');
  assert.equal(await page.locator('#local-update').isEnabled(),true);
  mode='invalid';await choose();
  await page.waitForFunction(()=>!document.querySelector('#local-update-error').hidden);
  assert.equal(await page.locator('#local-update-error').textContent(),'更新套件的版本資訊無效。');
  assert.equal(await page.locator('#local-update-install').isDisabled(),true);
  await page.locator('#local-update-cancel').click();
  await page.waitForFunction(()=>!document.querySelector('#local-update-dialog').open);
  mode='slow';await choose();
  await page.waitForFunction(()=>state.updates.local?.phase==='validating');
  await page.locator('#local-update-cancel').click();
  await page.waitForFunction(()=>!document.querySelector('#local-update-dialog').open);
  holdUpload();holdUpload=null;
  await page.waitForTimeout(100);
  assert.equal(state.updates.local,undefined,'讀取途中取消也要清除工作');
  mode='success';await choose();
  await page.waitForFunction(()=>document.querySelector('#local-update-install').disabled===false);
  // 重新整理後須能繼續確認或取消，不能永遠鎖住本地更新。
  await page.reload();
  await page.waitForFunction(()=>document.querySelector('#local-update-dialog').open);
  assert.equal(await page.locator('#local-update-install').isEnabled(),true);
  await page.locator('#local-update-install').click();
  await page.waitForFunction(()=>state.updates.local?.phase==='installing');
  assert.equal(writes.filter(x=>x[1].endsWith('/install')).length,1);
  assert.equal(await page.locator('#local-update-install').isDisabled(),true);
  assert.equal(await page.locator('#local-update-cancel').isDisabled(),true);
  await page.keyboard.press('Escape');
  assert.equal(await page.locator('#local-update-dialog').evaluate(el=>el.open),true,'交接中不可取消或重複安裝');
  fs.mkdirSync(path.resolve(__dirname,'../.local-run'),{recursive:true});
  await page.screenshot({path:path.resolve(__dirname,`../.local-run/local-update-${platform}.png`)});
  assert.equal(external,0,'本地更新不得連線下載');assert.deepEqual(errors,[]);
  await page.close();
 }
 console.log('PASS Windows ZIP／macOS DMG、四語與窄視窗排版、原始檔上傳、回退確認、取消與讀取中取消、驗證失敗、重新整理復原、明確安裝及防止重複交接；無外部網路請求');
}finally{await browser.close();}})().catch(error=>{console.error(error);process.exitCode=1;});
