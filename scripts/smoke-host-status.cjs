// 使用實際 HTML／JS 與隔離 API；不連遠端、不變更服務或使用者設定。
const fs=require('fs'),path=require('path'),assert=require('assert'),{chromium}=require('playwright');
const root=path.resolve(__dirname,'../internal/clientui/web'),origin='https://yourdesk-host-status.test';
(async()=>{const browser=await chromium.launch({headless:true});try{
 const page=await browser.newPage({viewport:{width:1040,height:720}}),errors=[];
 page.on('pageerror',error=>errors.push(error.message));
 const state={info:{room:'YD-USER',hostname:'Windows Smoke',platform:'windows',architecture:'amd64',version:'1.26.1015 build 1200'},library:{groups:[],sites:[]},preferences:{language:'zh-Hant',theme:'light',selectedGroup:'*'},running:{},sessions:{},updates:{},hardwareDetection:{status:'complete'},audioCapabilities:[],prelogin:{supported:true,desktopAccessSupported:true,installed:true,enabled:true,room:'YD-SERVICE'}};
 let local={room:'YD-SERVICE',online:false},failLocal=false,writes=0;
 await page.route('**/*',route=>{
  const request=route.request(),url=new URL(request.url());if(url.origin!==origin)return route.abort();
  if(url.pathname.startsWith('/api/')){
   if(request.method()!=='GET')writes++;
   if(url.pathname==='/api/presence/local')return route.fulfill({status:failLocal?500:200,json:failLocal?{error:'查詢失敗'}:local});
   return route.fulfill({json:url.pathname==='/api/state'?state:{}});
  }
  const filename=url.pathname==='/'?'index.html':url.pathname.slice(1),file=path.resolve(root,filename);
  if(!file.startsWith(root+path.sep)||!fs.existsSync(file))return route.abort();
  return route.fulfill({body:fs.readFileSync(file),contentType:({'.html':'text/html','.js':'text/javascript','.css':'text/css','.json':'application/json'})[path.extname(file)]||'application/octet-stream'});
 });
 await page.goto(origin+'/#smoke');
 await page.waitForFunction(()=>document.querySelector('#host-status').textContent==='配對服務未連線');
 const badge=page.locator('.local-badge'),label=page.locator('#host-status');
 const refresh=async()=>{await page.waitForFunction(()=>!checkingPresence);await page.evaluate(()=>refreshPresence());};
 const update=()=>page.evaluate(()=>updateRunning());
 assert.equal(await badge.getAttribute('data-state'),'offline','安裝服務不能假裝在線');
 assert.match(await badge.getAttribute('data-tooltip'),/登入前啟動已開啟/);
 state.prelogin.enabled=false;await update();
 assert.equal(await label.textContent(),'配對服務未連線');assert.match(await badge.getAttribute('data-tooltip'),/系統操作服務模式/);
 state.incomingConnected=true;await update();
 assert.equal(await label.textContent(),'遠端已連線','P2P 連線不能被中央服務查詢失敗蓋掉');
 assert.equal(await badge.getAttribute('data-state'),'connected');
 state.incomingConnected=false;state.prelogin.room='YD-NEW';await update();
 assert.equal(await label.textContent(),'正在確認連線狀態…','不能沿用另一個 ID 的在線狀態');
 local={room:'YD-SERVICE',online:true};await refresh();
 assert.equal(await label.textContent(),'正在確認連線狀態…','延遲回覆不能污染新 ID');
 local={room:'YD-NEW',online:null};await refresh();
 assert.equal(await label.textContent(),'無法確認連線狀態');
 failLocal=true;await refresh();
 assert.equal(await label.textContent(),'無法確認連線狀態','查詢失敗與明確離線必須區分');failLocal=false;
 state.prelogin={supported:true,enabled:false};state.running={host:true};await update();
 local={room:'YD-USER',online:false};await refresh();
 assert.equal(await label.textContent(),'配對服務未連線','普通 Host 有程序不等於已註冊');
 local={room:'YD-USER',online:true};await refresh();assert.equal(await label.textContent(),'一切就緒');
 state.prelogin={supported:true,installed:true,enabled:true,room:'YD-SERVICE'};await update();
 local={room:'YD-SERVICE',online:true,capabilities:{schema:1,version:'1.26.0923 build 1102'}};await refresh();
 assert.equal(await label.textContent(),'一切就緒');
 assert.match(await badge.getAttribute('data-tooltip'),/1\.26\.0923 build 1102/);
 assert.match(await badge.getAttribute('data-tooltip'),/手動替換 ZIP 不會更新已安裝的服務副本/);
 const translations=JSON.parse(fs.readFileSync(path.join(root,'translations.json'),'utf8'));
 for(const [language,index] of [['en',0],['ja',1],['ko',2]]){
  local.online=false;await refresh();
  await page.evaluate(language=>{i18n.apply(language);renderDevice();},language);
  assert.equal(await label.textContent(),translations['配對服務未連線'][index]);
 }
 await page.evaluate(()=>{i18n.apply('zh-Hant');renderDevice();});
 fs.mkdirSync(path.resolve(__dirname,'../.local-run'),{recursive:true});
 await page.screenshot({path:path.resolve(__dirname,'../.local-run/service-offline-status.png')});
 assert.equal(writes,0,'背景狀態查詢不可更改偏好或服務');assert.deepEqual(errors,[]);
 console.log('PASS 已安裝但離線、執行期間服務、P2P 優先、ID 切換與延遲回覆、未知／失敗分離、普通 Host、服務版本差異、四語顯示，且不修改設定或服務');
}finally{await browser.close()}})().catch(error=>{console.error(error);process.exitCode=1});
