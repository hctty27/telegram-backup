import {useState} from "react"
import {useQuery,useQueryClient} from "@tanstack/react-query"
import {Card} from "@/components/ui/card"

type Row=Record<string,string|number|boolean|null>
type Login={state:string;qr?:string;expires?:string}
const control="rounded-lg border border-border bg-background px-3 py-2 text-sm"
const button="rounded-lg border border-border bg-card px-3 py-2 text-sm font-medium hover:bg-background"
const primary="rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700"

async function request<T>(token:string,path:string,method="GET",body?:unknown):Promise<T>{
 const r=await fetch("/api/v2/"+path,{method,headers:{Authorization:"Bearer "+token,...(body===undefined?{}:{"Content-Type":"application/json"})},body:body===undefined?undefined:JSON.stringify(body),cache:"no-store"})
 if(!r.ok)throw new Error(r.status===401?"管理令牌无效":r.status===409?"当前状态不允许此操作":r.status===404?"工作引擎未启用":"操作失败，请检查连接与配置（"+r.status+"）")
 return r.json()
}
export function useCapabilities(token:string){
 return useQuery({queryKey:["capabilities"],queryFn:()=>request<{workers_enabled:boolean}>(token,"capabilities"),retry:false})
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
 const [apiID,setAPIID]=useState(""),[hash,setHash]=useState(""),[name,setName]=useState(""),[account,setAccount]=useState(""),[loginID,setLoginID]=useState(""),[password,setPassword]=useState("")
 const a=useAction()
 const login=useQuery({queryKey:["login",loginID],queryFn:()=>request<Login>(token,"accounts/"+loginID+"/qr"),enabled:!!loginID,refetchInterval:q=>["authorized","failed"].includes(q.state.data?.state||"")?false:1500})
 const status=login.data
 return <Card className="space-y-4 p-5"><h2 className="font-medium">添加与授权 Telegram 账号</h2><form className="grid gap-3 sm:grid-cols-2" onSubmit={e=>{e.preventDefault();void a.run(async()=>{
  const result=await request<{id:string}>(token,"accounts","POST",{api_id:Number(apiID),api_hash:hash,name})
  setHash("");setAPIID("");setName("");setAccount(result.id);return "账号已创建，请点击扫码授权。"
 })}}>
 <label className="grid gap-1 text-sm">账号备注<input className={control} value={name} onChange={e=>setName(e.target.value)} required maxLength={200}/></label>
 <label className="grid gap-1 text-sm">API ID<input className={control} type="number" min={1} value={apiID} onChange={e=>setAPIID(e.target.value)} required/></label>
 <label className="grid gap-1 text-sm sm:col-span-2">API Hash<input className={control} type="password" autoComplete="off" value={hash} onChange={e=>setHash(e.target.value)} required minLength={32} maxLength={32}/></label>
 <button disabled={a.busy} className={primary}>创建账号</button></form>
 <div className="flex flex-wrap gap-2 border-t border-border pt-4"><select aria-label="选择 Telegram 账号" className={control} value={account} onChange={e=>{setAccount(e.target.value);setLoginID("");setPassword("")}}><option value="">选择账号</option>{rows.map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.display_name||r.id)}</option>)}</select>
 <button className={button} disabled={!account||a.busy} onClick={()=>void a.run(async()=>{await request(token,"accounts/"+account+"/qr","POST");setLoginID(account);return "扫码授权已启动"})}>扫码授权</button>
 <button className={button} disabled={!account||a.busy} onClick={()=>void a.run(async()=>{const d=await request<{imported:number}>(token,"accounts/"+account+"/chats","POST");return "已刷新 "+d.imported+" 个会话"})}>刷新群组</button></div>
 {loginID&&<div className="space-y-3 rounded-lg border border-border p-4"><p className="text-sm">授权状态：{status?.state||"连接中"}</p>
 {status?.state==="waiting"&&status.qr&&<><img className="size-56 rounded-md bg-white p-2" src={status.qr} alt="Telegram 登录二维码"/><p className="text-sm text-muted-foreground">Telegram → 设置 → 设备 → 连接桌面设备；二维码自动刷新。</p></>}
 {status?.state==="password"&&<form className="flex flex-wrap gap-2" onSubmit={e=>{e.preventDefault();const p=password;setPassword("");void a.run(async()=>{await request(token,"accounts/"+loginID+"/password","POST",{password:p});return "已提交二次验证"})}}><input aria-label="二次验证密码" className={control} type="password" value={password} autoComplete="off" onChange={e=>setPassword(e.target.value)} required/><button className={primary} disabled={a.busy}>验证</button></form>}
 {status?.state==="authorized"&&<p className="text-sm text-emerald-600">已授权，可刷新群组。</p>}
 {status?.state==="failed"&&<p className="text-sm text-red-500">授权失败，请重新发起扫码。</p>}
 </div>}{a.message}</Card>
}
function Storage({token,rows}:{token:string;rows:Row[]}){
 const [name,setName]=useState(""),[clientID,setClientID]=useState(""),[secret,setSecret]=useState(""),[oauth,setOAuth]=useState(""),[folder,setFolder]=useState(""),[selected,setSelected]=useState(""),a=useAction()
 return <Card className="space-y-4 p-5">
 <h2 className="font-medium">添加 Google Drive</h2>
 <p className="text-sm text-muted-foreground">使用自己的 Google OAuth 桌面客户端。在有浏览器的电脑通过 rclone authorize drive 获取授权 Token，再填写下方信息。凭据在数据库内加密保存。</p>
 <a href="https://rclone.org/drive/#making-your-own-client-id" target="_blank" rel="noreferrer" className="text-sm text-primary">查看官方 OAuth 配置指南</a>
 <form className="grid gap-3 sm:grid-cols-2" onSubmit={e=>{e.preventDefault();void a.run(async()=>{
  await request(token,"storage","POST",{name,client_id:clientID,client_secret:secret,token:oauth,root_folder_id:folder});setSecret("");setOAuth("");setName("");setClientID("");setFolder("");return "连接测试通过，Google Drive 已加密保存"
 })}}>
 <label className="grid gap-1 text-sm">远端名称<input className={control} value={name} onChange={e=>setName(e.target.value)} required pattern="[A-Za-z0-9][A-Za-z0-9_.-]{0,63}" placeholder="drive"/></label>
 <label className="grid gap-1 text-sm">根文件夹 ID（可选）<input className={control} value={folder} onChange={e=>setFolder(e.target.value)} maxLength={256}/></label>
 <label className="grid gap-1 text-sm sm:col-span-2">OAuth Client ID<input className={control} value={clientID} onChange={e=>setClientID(e.target.value)} required maxLength={256}/></label>
 <label className="grid gap-1 text-sm sm:col-span-2">OAuth Client Secret<input className={control} type="password" autoComplete="off" value={secret} onChange={e=>setSecret(e.target.value)} required maxLength={256}/></label>
 <label className="grid gap-1 text-sm sm:col-span-2">rclone OAuth Token JSON<textarea className={control+" min-h-28 font-mono"} autoComplete="off" spellCheck={false} value={oauth} onChange={e=>setOAuth(e.target.value)} required maxLength={32768}/></label>
 <button className={primary} disabled={a.busy}>测试并保存</button>
 </form>
 <div className="flex flex-wrap gap-2 border-t border-border pt-4"><select className={control} value={selected} onChange={e=>setSelected(e.target.value)} aria-label="选择云存储"><option value="">选择已配置的远端</option>{rows.map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.name)}</option>)}</select><button className={button} disabled={!selected||a.busy} onClick={()=>void a.run(async()=>{await request(token,"storage/"+selected+"/test","POST");return "连接测试通过"})}>测试连接</button></div>{a.message}</Card>
}
function Chats({token,rows}:{token:string;rows:Row[]}){
 const [chat,setChat]=useState(""),[remote,setRemote]=useState(""),[interval,setInterval]=useState(300),[enabled,setEnabled]=useState(true),a=useAction()
 const remotes=useQuery({queryKey:["storage"],queryFn:()=>request<Row[]>(token,"storage")})
 const current=rows.find(r=>String(r.id)===chat)
 return <Card className="space-y-3 p-5"><h2 className="font-medium">备份策略</h2><form className="flex flex-wrap items-end gap-3" onSubmit={e=>{e.preventDefault();void a.run(async()=>{await request(token,"chats/"+chat+"/policy","PUT",{remote_id:remote,enabled,scan_interval_seconds:interval});return enabled?"策略已启用，将自动扫描历史和新增消息":"已暂停新扫描；已入队的传输仍会完成"})}}>
 <label className="grid gap-1 text-sm">群组<select className={control} value={chat} onChange={e=>{const id=e.target.value;setChat(id);const row=rows.find(r=>String(r.id)===id);setRemote(String(row?.remote_id||""));setInterval(Number(row?.scan_interval_seconds||300));setEnabled(row?.remote_id?Boolean(row.enabled):true)}} required><option value="">选择群组</option>{rows.map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.title||r.id)}</option>)}</select></label>
 <label className="grid gap-1 text-sm">云存储<select className={control} value={remote} onChange={e=>setRemote(e.target.value)} required disabled={!!current?.remote_id}><option value="">选择远端</option>{(remotes.data||[]).filter(r=>r.enabled).map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.name)}</option>)}</select></label>
 <label className="grid gap-1 text-sm">扫描间隔（秒）<input className={control+" w-32"} type="number" min={60} max={86400} value={interval} onChange={e=>setInterval(Number(e.target.value))} required/></label>
 <label className="flex items-center gap-2 py-2 text-sm"><input type="checkbox" checked={enabled} onChange={e=>setEnabled(e.target.checked)}/>自动备份</label><button className={primary} disabled={!chat||!remote||a.busy}>保存策略</button><button type="button" className={button} disabled={!current?.enabled||a.busy} onClick={()=>void a.run(async()=>{await request(token,"chats/"+chat+"/sync","POST");return "已请求立即扫描"})}>立即同步</button></form>
 <p className="text-xs text-muted-foreground">已启用的群组首次扫描全部历史，后续按游标增量同步。现有策略的远端不可直接更换。</p>{a.message}</Card>
}

function Tasks({token,rows}:{token:string;rows:Row[]}){
 const [id,setID]=useState(""),a=useAction()
 const failed=rows.filter(r=>r.queue_state==="discarded"||r.queue_state==="cancelled")
 if(failed.length===0)return null
 return <Card className="space-y-3 p-5"><h2 className="font-medium">失败任务恢复</h2><div className="flex flex-wrap gap-2"><select className={control} aria-label="选择失败任务" value={id} onChange={e=>setID(e.target.value)}><option value="">选择任务</option>{failed.map(r=><option key={String(r.id)} value={String(r.id)}>{String(r.filename)}</option>)}</select><button className={primary} disabled={!id||a.busy} onClick={()=>void a.run(async()=>{await request(token,"tasks/"+id+"/retry","POST");setID("");return "已重新入队"})}>重试</button></div>{a.message}</Card>
}
