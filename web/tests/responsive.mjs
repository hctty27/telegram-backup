import assert from "node:assert/strict";
import {createRequire} from "node:module";
import {join} from "node:path";
const dir=process.env.TGB_PLAYWRIGHT_DIR;
assert.ok(dir,"Set TGB_PLAYWRIGHT_DIR to the isolated playwright installation");
const {chromium,webkit,devices}=createRequire(join(dir,"package.json"))("playwright");
const token="browser-smoke-test-token-not-for-production";
const long="非常长的群组名称".repeat(18)+"_"+"x".repeat(160);
const account={id:"1",display_name:long,enabled:true,created_at:"2026-01-01T00:00:00Z"};
const group={id:"2",title:long,peer_kind:"channel",enabled:true,remote_id:"4",scan_interval_seconds:300};
const task={id:"3",filename:long+".mp4",state:"failed",expected_bytes:1234567890,queue_state:"discarded",attempt:10,last_error:long,updated_at:"2026-01-01T00:00:00Z"};
const remote={id:"4",name:"drive",kind:"drive",enabled:true};
const disk={staged_bytes:1024,free_bytes:10737418240,quota_bytes:10737418240,reserve_bytes:5368709120,promised_bytes:1024,available_bytes:5368709120};
const overview={accounts:1,chats:1,completed:1200,active:1,failed:1,staged_bytes:1024,verified_bytes:1234567890123};
const fixtures={overview,accounts:[account],chats:[group],tasks:[task],archive:[{...task,state:"completed"}],storage:[remote],capabilities:{workers_enabled:true,google_oauth:true}};
const cases=[
 ["compact 320px",chromium,{viewport:{width:320,height:720},isMobile:true,hasTouch:true}],
 ["iPhone Safari emulation",webkit,{...devices["iPhone 13"]}],
 ["Android Chrome emulation",chromium,{...devices["Pixel 7"]}],
 ["iPad WebKit emulation",webkit,{...devices["iPad Mini"]}],
 ["Desktop Chromium",chromium,{viewport:{width:1440,height:900}}],
];
async function noOverflow(page,label){
 const metrics=await page.evaluate(()=>({width:document.documentElement.clientWidth,scroll:document.documentElement.scrollWidth,body:document.body.scrollWidth}));
 assert.ok(Math.max(metrics.scroll,metrics.body)<=metrics.width+1,label+" horizontal overflow "+JSON.stringify(metrics));
}
for(const [label,type,options] of cases){
 const browser=await type.launch({headless:true});
 const context=await browser.newContext(options);
 const page=await context.newPage();
 page.setDefaultTimeout(10000);
 const errors=[];
 page.on("pageerror",error=>errors.push(error.message));
 let postAccount=false;
 await page.route("**/api/**",async route=>{
  const req=route.request();
  const name=new URL(req.url()).pathname.slice(5);
  if(req.headers().authorization!=="Bearer "+token)return route.fulfill({status:401,body:"unauthorized"});
  if(name==="monitor/events")return route.fulfill({status:200,contentType:"text/event-stream",body:"data: "+JSON.stringify({disk,transfers:[{id:"1",media_id:"3",stage:"download",bytes:1024,total:4096,speed:200}]})+"\n\n"});
  if(name==="accounts"&&req.method()==="POST"){
   const data=req.postDataJSON();
   postAccount=data.name==="Test account"&&data.api_id===123&&data.api_hash.length===32;
   return route.fulfill({json:{id:"1"}});
  }
  if(name in fixtures)return route.fulfill({json:fixtures[name]});
  return route.fulfill({status:404,body:"not found"});
 });
 try{
  await page.goto("http://127.0.0.1:4173/");
  await page.getByLabel("管理令牌").fill(token);
  await noOverflow(page,label+" login");
  await page.getByRole("button",{name:"进入控制台"}).click();
  await page.getByText("已归档文件",{exact:true}).waitFor();
  await page.getByText("磁盘保护",{exact:true}).waitFor();
  assert.equal(await page.getByText("开发版").count(),0,label+" development badge");
  await noOverflow(page,label+" overview");
  const mobile=await page.evaluate(()=>matchMedia("(max-width:767px)").matches);
  const nav=page.getByRole("navigation",{name:"主导航"});
  const theme=page.getByRole("button",{name:"切换主题"});
  await theme.click();
  await page.waitForFunction(()=>document.documentElement.classList.contains("dark")===(getComputedStyle(document.documentElement).colorScheme==="dark"));
  await noOverflow(page,label+" theme");
  const pages=[["Telegram 账号","添加与授权 Telegram 账号"],["群组管理","备份策略"],["任务中心","失败任务恢复"],["媒体归档",null],["云存储","添加 Google Drive"]];
  for(const [name,heading] of pages){
   if(mobile)await page.getByRole("button",{name:"打开导航"}).click();
   await nav.getByRole("button",{name,exact:true}).click();
   await page.getByRole("heading",{name,exact:true,level:1}).waitFor();
   if(heading)await page.getByRole("heading",{name:heading,exact:true}).waitFor();
   if(mobile){
    assert.equal(await nav.isVisible(),false,label+" nav did not close");
    await page.getByTestId("mobile-records").waitFor({state:"visible"});
   }else await page.locator("table").waitFor({state:"visible"});
   await noOverflow(page,label+" "+name);
   if(name==="Telegram 账号"){
    await page.getByLabel("账号备注").fill("Test account");
    await page.getByLabel("API ID",{exact:true}).fill("123");
    await page.getByLabel("API Hash").fill("a".repeat(32));
    await page.getByRole("button",{name:"创建账号",exact:true}).click();
    await page.getByText("账号已创建，请点击扫码授权。",{exact:true}).waitFor();
    assert.ok(postAccount,label+" account form payload");
    await noOverflow(page,label+" account feedback");
   }
  }
  await page.getByRole("button",{name:"退出",exact:true}).click();
  await page.getByRole("button",{name:"进入控制台"}).waitFor();
  assert.deepEqual(errors,[],label+" page errors");
  console.log("PASS "+label);
 }finally{await context.close();await browser.close()}
}
