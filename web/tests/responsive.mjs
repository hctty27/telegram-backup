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
const version={service:"telegram-backup",version:"main-123456789abc",commit:"123456789abcdef0123456789abcdef012345678",build_time:"2026-11-28T00:00:00Z",go_version:"go1.27.4",platform:"linux/amd64"};
const fixtures={version,overview,accounts:[account],chats:[group],tasks:[task],archive:[{...task,state:"completed"}],storage:[remote],capabilities:{workers_enabled:true,google_oauth:true}};
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
 let loginState={state:"idle"};
 let codeTries=0;
 let passwordTries=0;
 let resent=false;
 const qrFixture="data:image/svg+xml,"+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="320" height="320" viewBox="0 0 320 320"><rect width="320" height="320" fill="white"/><rect x="32" y="32" width="256" height="256" fill="black"/></svg>');
 await page.route("**/api/**",async route=>{
  const req=route.request();
  const name=new URL(req.url()).pathname.slice(5);
  if(req.headers().authorization!=="Bearer "+token)return route.fulfill({status:401,body:"unauthorized"});
  if(name==="monitor/events")return route.fulfill({status:200,contentType:"text/event-stream",body:"data: "+JSON.stringify({disk,transfers:[{id:"1",media_id:"3",stage:"download",bytes:1024,total:4096,speed:200}]})+"\n\n"});
  if(name==="accounts/1/login")return route.fulfill({json:loginState});
  if(name.startsWith("accounts/1/")&&req.method()==="POST"){
   const action=name.split("/").pop();
   const body=req.postData()?req.postDataJSON():{};
   if(action==="qr")loginState={state:"waiting",method:"qr",attempt_id:"qr-test",qr:qrFixture};
   else if(action==="phone"){
    assert.equal(body.phone,"+12025550123");
    loginState={state:"code",method:"phone",attempt_id:"phone-test",channel:"app",code_length:5,can_resend:true,resend_at:"2000-01-01T00:00:00Z"};
   }else{
    assert.equal(body.attempt_id,loginState.attempt_id);
    if(action==="cancel")loginState={state:"cancelled",attempt_id:body.attempt_id};
    else if(action==="resend"){resent=true;loginState={...loginState,channel:"sms",can_resend:false};}
    else if(action==="code"){
     codeTries++;
     if(body.code==="00000")loginState={...loginState,error:"invalid_code"};
     else{assert.equal(body.code,"12345");loginState={state:"password",attempt_id:body.attempt_id,method:"phone"};}
    }else if(action==="password"){
     passwordTries++;
     if(body.password==="wrong-test")loginState={...loginState,error:"invalid_password"};
     else{assert.equal(body.password,"correct-test");loginState={state:"authorized",attempt_id:body.attempt_id,method:"phone"};}
    }else return route.fulfill({status:404,body:"not found"});
   }
   return route.fulfill({json:loginState});
  }
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
  await page.getByText(version.version,{exact:true}).waitFor();
  await page.getByText(version.commit,{exact:true}).waitFor();
  await noOverflow(page,label+" overview");
  const mobile=await page.evaluate(()=>matchMedia("(max-width:1023px)").matches);
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
    await page.getByText("账号已创建，请选择授权方式。",{exact:true}).waitFor();
    assert.ok(postAccount,label+" account form payload");
    await noOverflow(page,label+" account feedback");
    await page.getByRole("button",{name:"扫码授权",exact:true}).click();
    await page.getByText("授权状态：等待扫码",{exact:true}).waitFor();
    const image=page.getByRole("img",{name:"Telegram 登录二维码"});
    await image.waitFor({state:"visible"});
    await page.waitForFunction(()=>{const img=document.querySelector(".login-qr");return img?.complete&&img.naturalWidth>0;});
    const rect=await image.boundingBox();
    assert.ok(rect&&rect.width>=180&&rect.width<=257&&Math.abs(rect.width-rect.height)<=1,label+" incorrect QR layout "+JSON.stringify(rect));
    await noOverflow(page,label+" QR");
    await page.getByRole("button",{name:"取消授权",exact:true}).click();
    await page.getByText("授权状态：已取消",{exact:true}).waitFor();
    await page.getByRole("button",{name:"手机号登录",exact:true}).click();
    await page.getByLabel("手机号（含国际区号）").fill("+12025550123");
    await page.getByRole("button",{name:"发送验证码",exact:true}).click();
    await page.getByText("授权状态：等待验证码",{exact:true}).waitFor();
    await page.getByText("请查看已登录 Telegram 客户端内的官方登录通知。",{exact:true}).waitFor();
    await noOverflow(page,label+" phone");
    await page.getByRole("button",{name:"重新发送验证码",exact:true}).click();
    await page.getByText("请查看手机短信。",{exact:true}).waitFor();
    await page.getByLabel("Telegram 验证码",{exact:true}).fill("00000");
    await page.getByRole("button",{name:"验证验证码",exact:true}).click();
    await page.getByText("验证码错误，请重新输入。",{exact:true}).waitFor();
    await page.getByLabel("Telegram 验证码",{exact:true}).fill("12345");
    await page.getByRole("button",{name:"验证验证码",exact:true}).click();
    await page.getByText("授权状态：等待二次验证",{exact:true}).waitFor();
    await page.getByLabel("二次验证密码").fill("wrong-test");
    await page.getByRole("button",{name:"验证",exact:true}).click();
    await page.getByText("二次验证密码错误，请重试。",{exact:true}).waitFor();
    await page.getByLabel("二次验证密码").fill("correct-test");
    await page.getByRole("button",{name:"验证",exact:true}).click();
    await page.getByText("授权状态：已授权",{exact:true}).waitFor();
    assert.ok(resent&&codeTries===2&&passwordTries===2,label+" phone flow");
    await noOverflow(page,label+" authorized");
   }
  }
  await page.getByRole("button",{name:"退出",exact:true}).click();
  await page.getByRole("button",{name:"进入控制台"}).waitFor();
  assert.deepEqual(errors,[],label+" page errors");
  console.log("PASS "+label);
 }finally{await context.close();await browser.close()}
}
