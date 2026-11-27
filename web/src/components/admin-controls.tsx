import {useEffect,useState} from "react"
import {useQuery,useQueryClient} from "@tanstack/react-query"
import {Card} from "@/components/ui/card"

type Row=Record<string,string|number|boolean|null>
type Login={state:string;attempt_id?:string;method?:string;qr?:string;expires?:string;deadline?:string;channel?:string;code_length?:number;can_resend?:boolean;resend_at?:string;error?:string;retry_at?:string}
const loginLabels:Record<string,string>={idle:"未授权",connecting:"正在连接",waiting:"等待扫码",code:"等待验证码",password:"等待二次验证",checking:"正在验证",cancelling:"正在取消",cancelled:"已取消",expired:"已超时",authorized:"已授权",failed:"授权失败"}
const loginErrors:Record<string,string>={invalid_code:"验证码错误，请重新输入。",invalid_password:"二次验证密码错误，请重试。",invalid_phone:"手机号无效，请检查国际区号。",phone_banned:"Telegram 拒绝此手机号登录。",code_expired:"验证码已失效，请重新发起登录。",rate_limited:"Telegram 限制登录频率，请在限制结束后重试。",unsupported_channel:"Telegram 要求当前未支持的验证方式，请改用扫码授权。",resend_unavailable:"Telegram 没有可用的重发渠道，请改用扫码或稍后重试。",signup_required:"该号码尚未注册 Telegram，请先在官方客户端注册。",restart_required:"Telegram 要求重新发起授权。",identity_changed:"该账号记录已绑定其他 Telegram 用户，请新建账号记录。",attempts_exhausted:"尝试次数过多，请重新发起授权。",timeout:"授权超时，请重新发起。",login_failed:"授权失败，请检查连接、API 凭据或改用另一种登录方式。"}
const channelLabels:Record<string,string>={app:"请查看已登录 Telegram 客户端内的官方登录通知。",sms:"请查看手机短信。",call:"请接听 Telegram 的验证码语音电话。",missed_call:"请输入 Telegram 来电号码末尾指定长度的数字。"}
const activeLogin=(state?:string)=>["connecting","waiting","code","password","checking","cancelling"].includes(state||"")
const control="form-control min-w-0 max-w-full rounded-lg border border-border bg-background px-3 py-2 text-base md:text-sm"
const button="min-h-11 max-w-full rounded-lg border border-border bg-card px-3 py-2 text-sm font-medium hover:bg-background"
const primary="min-h-11 max-w-full rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"

async function request<T>(token:string,path:string,method="GET",body?:unknown):Promise<T>{
 const r=await fetch("/api/"+path,{method,headers:{Authorization:"Bearer "+token,...(body===undefined?{}:{"Content-Type":"application/json"})},body:body===undefined?undefined:JSON.stringify(body),cache:"no-store"})
 if(!r.ok)throw new Error(r.status===401?"管理令牌无效":r.status===409?"当前状态不允许此操作":r.status===404?"工作引擎未启用":"操作失败，请检查连接与配置（"+r.status+"）")
 return r.json()
}
export function useCapabilities(token:string){
 return useQuery({queryKey:["capabilities"],queryFn:()=>request<{workers_enabled:boolean;google_oauth?:boolean}>(token,"capabilities"),retry:false})
}

export function AdminControls({page,token,rows}:{page:string;token:string;rows:Row[]}){
 const caps=useCapabilities(token)
 if(!caps.data?.workers_enabled)return null
 if(page==="accounts")return <Accounts token={token} rows={rows}/>
 if(page==="storage")return <Storage token={token} rows={rows}/>
 if(page==="chats")return <Chats token={token} rows={rows}/>
 if(page==="tasks")return <Tasks token={token} rows={rows}/>
 return null
}
function useAction(){
 const qc=useQueryClient()
 const [busy,setBusy]=useState(false),[notice,setNotice]=useState(""),[error,setError]=useState(false)
 async function run(fn:()=>Promise<string>){
  if(busy)return
  setBusy(true);setNotice("");setError(false)
  try{setNotice(await fn());await qc.invalidateQueries()}catch(e){setError(true);setNotice(e instanceof Error?e.message:"操作失败")}finally{setBusy(false)}
 }
 return {busy,run,message:notice&&<p role="status" className={error?"text-sm text-red-500":"text-sm text-muted-foreground"}>{notice}</p>}
}
function Accounts({token,rows}:{token:string;rows:Row[]}){
 const [apiID,setAPIID]=useState(""),[hash,setHash]=useState(""),[name,setName]=useState(""),[account,setAccount]=useState(""),[method,setMethod]=useState<"qr"|"phone">("qr"),[phone,setPhone]=useState(""),[code,setCode]=useState(""),[password,setPassword]=useState(""),[now,setNow]=useState(Date.now)
 const a=useAction(),qc=useQueryClient()
 const key=["login",account]
 const login=useQuery({queryKey:key,queryFn:()=>request<Login>(token,"accounts/"+account+"/login"),enabled:!!account,refetchInterval:q=>activeLogin(q.state.data?.state)?1500:false})
 const status=login.data,active=activeLogin(status?.state),retry=Math.max(0,Math.ceil((Date.parse(status?.retry_at||"")-now)/1000)||0),resend=Math.max(0,Math.ceil((Date.parse(status?.resend_at||"")-now)/1000)||0)
 useEffect(()=>{const timer=window.setInterval(()=>setNow(Date.now()),1000);return ()=>window.clearInterval(timer)},[])
 useEffect(()=>{if(status?.state==="authorized")void qc.invalidateQueries({queryKey:["accounts"]})},[status?.state,qc])
 const clearSecrets=()=>{setPhone("");setCode("");setPassword("")}
 async function perform(action:string,body?:unknown){
  const result=await request<Login>(token,"accounts/"+account+"/"+action,"POST",body)
  qc.setQueryData(key,result)
 }
 function submit(action:string,body:Record<string,string>,message:string){
  if(!status?.attempt_id)return
  const attempt_id=status.attempt_id
  void a.run(async()=>{await perform(action,{attempt_id,...body});return message})
 }
 return <Card className="admin-card min-w-0 space-y-4 p-4 sm:p-5"><h2 className="font-medium">添加与授权 Telegram 账号</h2><form className="grid gap-3 sm:grid-cols-2" onSubmit={e=>{e.preventDefault();void a.run(async()=>{
  const result=await request<{id:string}>(token,"accounts","POST",{api_id:Number(apiID),api_hash:hash,name})
  setHash("");setAPIID("");setName("");setAccount(result.id);clearSecrets();return "账号已创建，请选择授权方式。"
 })}}>
 <label className="grid min-w-0 gap-1 text-sm">账号备注<input className={control} value={name} onChange={e=>setName(e.target.value)} required maxLength={200}/></label>
 <label className="grid min-w-0 gap-1 text-sm">API ID<input className={control} type="number" min={1} value={apiID} onChange={e=>setAPIID(e.target.value)} required/></label>
 <label className="grid min-w-0 gap-1 text-sm sm:col-span-2">API Hash<input className={control} type="password" autoComplete="off" value={hash} onChange={e=>setHash(e.target.value)} required minLength={32} maxLength={32}/></label>
 <button disabled={a.busy||active} className={primary}>创建账号</button></form>
 <div className="space-y-3 border-t border-border pt-4"><select aria-label="选择 Telegram 账号" className={control+" w-full"} value={account} disabled={a.busy||active} onChange={e=>{setAccount(e.target.value);clearSecrets()}}><option value="">选择账号</option>{rows.map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.display_name||r.id)}</option>)}</select>
 {account&&<div className="grid grid-cols-2 gap-2" role="group" aria-label="授权方式"><button className={method==="qr"?primary:button} aria-pressed={method==="qr"} disabled={active||a.busy} onClick={()=>{setMethod("qr");clearSecrets()}}>二维码登录</button><button className={method==="phone"?primary:button} aria-pressed={method==="phone"} disabled={active||a.busy} onClick={()=>{setMethod("phone");clearSecrets()}}>手机号登录</button></div>}
 {!active&&account&&method==="qr"&&<button className={button} disabled={a.busy||login.isPending||retry>0} onClick={()=>void a.run(async()=>{await perform("qr");return "扫码授权已启动"})}>扫码授权</button>}
 {!active&&account&&method==="phone"&&<form className="grid gap-2 sm:flex" onSubmit={e=>{e.preventDefault();const value=phone.trim();setPhone("");void a.run(async()=>{await perform("phone",{phone:value});return "已请求 Telegram 发送验证码"})}}><label className="grid min-w-0 flex-1 gap-1 text-sm">手机号（含国际区号）<input className={control} type="tel" autoComplete="tel" value={phone} onChange={e=>setPhone(e.target.value)} required pattern="[+][1-9][0-9]{6,14}" placeholder="+8613800000000"/></label><button className={primary+" self-end"} disabled={a.busy||login.isPending||retry>0}>发送验证码</button></form>}
 <button className={button} disabled={!account||a.busy||active} onClick={()=>void a.run(async()=>{const d=await request<{imported:number}>(token,"accounts/"+account+"/chats","POST");return "已刷新 "+d.imported+" 个会话"})}>刷新群组</button>
 </div>
 {account&&login.isError&&<p className="text-sm text-red-500">授权状态查询失败，请刷新重试。</p>}
 {account&&status&&status.state!=="idle"&&<div className="space-y-3 rounded-lg border border-border p-4"><p className="text-sm">授权状态：{loginLabels[status.state]||"未知状态"}</p>
 {status.error&&<p role="alert" className="text-sm text-red-500">{loginErrors[status.error]||"授权失败，请重试。"}</p>}
 {retry>0&&<p className="text-sm text-muted-foreground">请在 {retry} 秒后重试。</p>}
 {status.state==="waiting"&&status.qr&&<><img className="login-qr" src={status.qr} alt="Telegram 登录二维码" width={256} height={256}/><p className="text-sm text-muted-foreground">Telegram → 设置 → 设备 → 连接桌面设备；二维码自动刷新。</p></>}
 {status.state==="code"&&<><p className="text-sm text-muted-foreground">{channelLabels[status.channel||""]||"请输入 Telegram 提供的验证码。"}</p><form className="grid gap-2 sm:flex" onSubmit={e=>{e.preventDefault();const value=code;setCode("");submit("code",{code:value},"已提交验证码")}}><input aria-label="Telegram 验证码" className={control+" flex-1"} inputMode="numeric" autoComplete="one-time-code" type="text" pattern="[0-9]+" minLength={status.code_length||1} maxLength={status.code_length||20} value={code} onChange={e=>setCode(e.target.value.replace(/[^0-9]/g,""))} required/><button className={primary} disabled={a.busy}>验证验证码</button></form>{status.can_resend?<button className={button} disabled={a.busy||resend>0} onClick={()=>{setCode("");submit("resend",{},"已请求重发验证码")}}>{resend>0?"重新发送（"+resend+"秒）":"重新发送验证码"}</button>:<p className="text-xs text-muted-foreground">Telegram 未提供可用的重发渠道。</p>}</>}
 {status.state==="password"&&<form className="grid gap-2 sm:flex" onSubmit={e=>{e.preventDefault();const value=password;setPassword("");submit("password",{password:value},"已提交二次验证")}}><input aria-label="二次验证密码" className={control+" flex-1"} type="password" value={password} autoComplete="off" onChange={e=>setPassword(e.target.value)} required maxLength={1024}/><button className={primary} disabled={a.busy}>验证</button></form>}
 {active&&<button className={button} disabled={a.busy||status.state==="cancelling"} onClick={()=>{clearSecrets();submit("cancel",{},"已请求取消授权")}}>取消授权</button>}
 {status.state==="authorized"&&<p className="text-sm text-emerald-600">已授权，可刷新群组。</p>}
 </div>}{a.message}</Card>
}
function Storage({token,rows}:{token:string;rows:Row[]}){
 const caps=useCapabilities(token)
 const [authorizeURL,setAuthorizeURL]=useState(""),[name,setName]=useState(""),[clientID,setClientID]=useState(""),[secret,setSecret]=useState(""),[oauth,setOAuth]=useState(""),[folder,setFolder]=useState(""),[selected,setSelected]=useState(""),a=useAction()
 return <Card className="admin-card min-w-0 space-y-4 p-4 sm:p-5">
 <h2 className="font-medium">添加 Google Drive</h2>
 <p className="text-sm text-muted-foreground">支持 Web OAuth 客户端授权，也可导入桌面 rclone 授权 Token。凭据在数据库内加密保存。</p>
 <a href="https://rclone.org/drive/#making-your-own-client-id" target="_blank" rel="noreferrer" className="text-sm text-primary">查看官方 OAuth 配置指南</a>
 <form className="grid gap-3 sm:grid-cols-2" onSubmit={e=>{e.preventDefault();void a.run(async()=>{
  await request(token,"storage","POST",{name,client_id:clientID,client_secret:secret,token:oauth,root_folder_id:folder});setSecret("");setOAuth("");setName("");setClientID("");setFolder("");return "连接测试通过，Google Drive 已加密保存"
 })}}>
 <label className="grid min-w-0 gap-1 text-sm">远端名称<input className={control} value={name} onChange={e=>setName(e.target.value)} required pattern="[A-Za-z0-9][A-Za-z0-9_.-]{0,63}" placeholder="drive"/></label>
 <label className="grid min-w-0 gap-1 text-sm">根文件夹 ID（可选）<input className={control} value={folder} onChange={e=>setFolder(e.target.value)} maxLength={256}/></label>
 <label className="grid min-w-0 gap-1 text-sm sm:col-span-2">OAuth Client ID<input className={control} value={clientID} onChange={e=>setClientID(e.target.value)} required maxLength={256}/></label>
 <label className="grid min-w-0 gap-1 text-sm sm:col-span-2">OAuth Client Secret<input className={control} type="password" autoComplete="off" value={secret} onChange={e=>setSecret(e.target.value)} required maxLength={256}/></label>
 <label className="grid min-w-0 gap-1 text-sm sm:col-span-2">rclone OAuth Token JSON<textarea className={control+" min-h-28 font-mono"} autoComplete="off" spellCheck={false} value={oauth} onChange={e=>setOAuth(e.target.value)} maxLength={32768}/></label>
 <div className="flex flex-wrap gap-2 sm:col-span-2"><button className={primary} disabled={a.busy||!oauth}>导入 Token 并保存</button>{caps.data?.google_oauth&&<button type="button" className={button} disabled={a.busy||!name||!clientID||!secret} onClick={()=>void a.run(async()=>{setAuthorizeURL("");const d=await request<{url:string}>(token,"storage/oauth/start","POST",{name,client_id:clientID,client_secret:secret,token:"",root_folder_id:folder});setAuthorizeURL(d.url);setSecret("");return "授权链接已生成，请打开链接完成 Google 授权"})}>生成 Google 授权链接</button>}</div>
 </form>{authorizeURL&&<div className="space-y-2 text-sm"><a href={authorizeURL} target="_blank" rel="noreferrer" className="text-primary underline">前往 Google 官方授权页面</a><p className="text-muted-foreground">完成授权后返回此页面，点击顶部刷新按钮查看已添加的远端。授权链接十分钟内有效。</p></div>}
 <div className="flex flex-wrap gap-2 border-t border-border pt-4"><select className={control} value={selected} onChange={e=>setSelected(e.target.value)} aria-label="选择云存储"><option value="">选择已配置的远端</option>{rows.map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.name)}</option>)}</select><button className={button} disabled={!selected||a.busy} onClick={()=>void a.run(async()=>{await request(token,"storage/"+selected+"/test","POST");return "连接测试通过"})}>测试连接</button></div>{a.message}</Card>
}
function Chats({token,rows}:{token:string;rows:Row[]}){
 const [chat,setChat]=useState(""),[remote,setRemote]=useState(""),[interval,setInterval]=useState(300),[enabled,setEnabled]=useState(true),a=useAction()
 const remotes=useQuery({queryKey:["storage"],queryFn:()=>request<Row[]>(token,"storage")})
 const current=rows.find(r=>String(r.id)===chat)
 return <Card className="admin-card min-w-0 space-y-3 p-4 sm:p-5"><h2 className="font-medium">备份策略</h2><form className="policy-form grid min-w-0 gap-3 sm:grid-cols-2 lg:flex lg:flex-wrap lg:items-end" onSubmit={e=>{e.preventDefault();void a.run(async()=>{await request(token,"chats/"+chat+"/policy","PUT",{remote_id:remote,enabled,scan_interval_seconds:interval});return enabled?"策略已启用，将自动扫描历史和新增消息":"已暂停新扫描；已入队的传输仍会完成"})}}>
 <label className="grid min-w-0 gap-1 text-sm">群组<select className={control} value={chat} onChange={e=>{const id=e.target.value;setChat(id);const row=rows.find(r=>String(r.id)===id);setRemote(String(row?.remote_id||""));setInterval(Number(row?.scan_interval_seconds||300));setEnabled(row?.remote_id?Boolean(row.enabled):true)}} required><option value="">选择群组</option>{rows.map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.title||r.id)}</option>)}</select></label>
 <label className="grid min-w-0 gap-1 text-sm">云存储<select className={control} value={remote} onChange={e=>setRemote(e.target.value)} required disabled={!!current?.remote_id}><option value="">选择远端</option>{(remotes.data||[]).filter(r=>r.enabled).map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.name)}</option>)}</select></label>
 <label className="grid min-w-0 gap-1 text-sm">扫描间隔（秒）<input className={control+" w-full lg:w-32"} type="number" min={60} max={86400} value={interval} onChange={e=>setInterval(Number(e.target.value))} required/></label>
 <label className="flex items-center gap-2 py-2 text-sm"><input type="checkbox" checked={enabled} onChange={e=>setEnabled(e.target.checked)}/>自动备份</label><button className={primary} disabled={!chat||!remote||a.busy}>保存策略</button><button type="button" className={button} disabled={!current?.enabled||a.busy} onClick={()=>void a.run(async()=>{await request(token,"chats/"+chat+"/sync","POST");return "已请求立即扫描"})}>立即同步</button></form>
 <p className="text-xs text-muted-foreground">已启用的群组首次扫描全部历史，后续按游标增量同步。现有策略的远端不可直接更换。</p>{a.message}</Card>
}

function Tasks({token,rows}:{token:string;rows:Row[]}){
 const [id,setID]=useState(""),a=useAction()
 const failed=rows.filter(r=>r.queue_state==="discarded"||r.queue_state==="cancelled")
 if(failed.length===0)return null
 return <Card className="admin-card min-w-0 space-y-3 p-4 sm:p-5"><h2 className="font-medium">失败任务恢复</h2><div className="flex flex-wrap gap-2"><select className={control} aria-label="选择失败任务" value={id} onChange={e=>setID(e.target.value)}><option value="">选择任务</option>{failed.map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.filename)}</option>)}</select><button className={primary} disabled={!id||a.busy} onClick={()=>void a.run(async()=>{await request(token,"tasks/"+id+"/retry","POST");setID("");return "已重新入队"})}>重试</button></div>{a.message}</Card>
}
