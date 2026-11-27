import React,{useState,useEffect} from "react"
import {createRoot} from "react-dom/client"
import {QueryClient,QueryClientProvider,useQuery,useQueryClient} from "@tanstack/react-query"
import {Activity,Archive,Cloud,Database,HardDrive,LayoutDashboard,ListChecks,LogOut,MessagesSquare,Moon,Send,Sun,Users,RefreshCw,ShieldCheck,AlertCircle,Menu,X} from "lucide-react"
import {Card} from "@/components/ui/card"
import {cn} from "@/lib/utils"
import {AdminControls,useCapabilities} from "@/components/admin-controls"
import {LiveMonitor} from "@/components/live-monitor"
import "./styles.css"

const client=new QueryClient({defaultOptions:{queries:{retry:false,refetchOnWindowFocus:true}}})
const pages=[{key:"overview",label:"总览",icon:LayoutDashboard},{key:"accounts",label:"Telegram 账号",icon:Users},{key:"chats",label:"群组管理",icon:MessagesSquare},{key:"tasks",label:"任务中心",icon:ListChecks},{key:"archive",label:"媒体归档",icon:Archive},{key:"storage",label:"云存储",icon:Cloud}] as const
type Page=typeof pages[number]["key"]
type Overview={accounts:number;chats:number;completed:number;active:number;failed:number;staged_bytes:number;verified_bytes:number}
type Row=Record<string,string|number|boolean|null>
const columns:Record<Exclude<Page,"overview">,[string,string][]>={accounts:[["id","ID"],["display_name","账号"],["enabled","启用"],["created_at","创建时间"]],chats:[["id","ID"],["title","群组名称"],["peer_kind","类型"],["enabled","启用"]],tasks:[["filename","文件"],["state","状态"],["expected_bytes","大小"],["queue_state","队列状态"],["attempt","尝试次数"],["last_error","错误"],["updated_at","更新时间"]],archive:[["filename","文件"],["state","状态"],["expected_bytes","大小"],["updated_at","更新时间"]],storage:[["name","存储名称"],["kind","类型"],["enabled","启用"]]}
function bytes(n:number){if(!Number.isFinite(n)||n<0)return "—";if(n===0)return "0 B";const unit=["B","KiB","MiB","GiB","TiB"];const i=Math.min(4,Math.floor(Math.log(n)/Math.log(1024)));return (n/1024**i).toFixed(i?1:0)+" "+unit[i]}
async function load<T,>(page:Page,token:string,signal:AbortSignal):Promise<T>{
 const res=await fetch("/api/"+page,{headers:{Authorization:"Bearer "+token},signal,cache:"no-store"})
 if(!res.ok)throw new Error(res.status===401?"管理令牌无效":"接口暂不可用（"+res.status+"）")
 return res.json()
}
function value(key:string,v:Row[string]){if(v===null||v===undefined)return "—";if(key.endsWith("_bytes"))return bytes(Number(v));if(key.endsWith("_at"))return new Date(String(v)).toLocaleString();if(typeof v==="boolean")return v?"已启用":"未启用";return String(v)}
function DataView({page,token}:{page:Page;token:string}){
 const caps=useCapabilities(token)
 const q=useQuery({queryKey:[page],queryFn:({signal})=>load<Overview|Row[]>(page,token,signal),refetchInterval:5000})
 if(q.isPending)return <div role="status" className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">{Array.from({length:4},(_,i)=><div key={i} className="h-32 animate-pulse rounded-xl border border-border bg-card"/>)}</div>
 if(q.isError)return <Card className="flex items-center gap-3 p-6"><AlertCircle className="size-5 text-red-500"/><div><p className="font-medium">{q.error.message}</p><button className="mt-2 text-sm text-primary" onClick={()=>void q.refetch()}>重试连接</button></div></Card>
 if(page==="overview"){
  const d=q.data as Overview
  const cards=[{label:"已归档文件",n:d.completed,icon:Archive,sub:"已完成完整归档"},{label:"云端备份",n:bytes(d.verified_bytes),icon:Cloud,sub:"已完成媒体的源文件大小"},{label:"启用群组",n:d.chats,icon:MessagesSquare,sub:d.accounts+" 个 Telegram 账号"},{label:"传输中",n:d.active,icon:Activity,sub:d.failed+" 个失败或受限任务"}]
  return <div className="space-y-5"><div className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">{cards.map(c=><Card key={c.label} className="min-w-0 p-3 sm:p-5"><div className="flex items-center justify-between gap-1 text-xs text-muted-foreground sm:text-sm"><span className="min-w-0">{c.label}</span><c.icon className="size-4 shrink-0"/></div><div className="mt-3 break-words text-xl font-semibold tracking-tight tabular-nums sm:text-3xl">{c.n}</div><p className="mt-2 text-xs text-muted-foreground">{c.sub}</p></Card>)}</div><div className="grid gap-4 lg:grid-cols-2"><Card className="p-5"><div className="flex items-center gap-2 font-medium"><HardDrive className="size-4"/>暂存文件</div><p className="mt-4 text-3xl font-semibold tabular-nums">{bytes(d.staged_bytes)}</p><p className="mt-2 text-sm text-muted-foreground">数据库记录的暂存大小，不代表整块磁盘使用量。</p></Card><Card className="p-5"><div className="flex items-center gap-2 font-medium"><Database className="size-4"/>数据库连接</div><p className="mt-4 flex items-center gap-2 text-sm"><span className="size-2 rounded-full bg-emerald-500"/>查询正常</p><p className="mt-2 text-sm text-muted-foreground">每 5 秒刷新；后台任务状态以数据库记录为准。</p></Card></div><LiveMonitor token={token} enabled={!!caps.data?.workers_enabled}/><Card className="p-5"><div className="flex gap-3"><ShieldCheck className="mt-0.5 size-5 text-primary"/><div><p className="font-medium">{caps.data?.workers_enabled?"自动备份已启用":"只读模式"}</p><p className="mt-1 text-sm leading-relaxed text-muted-foreground">{caps.data?.workers_enabled?"已授权且启用策略的群组接收实时媒体事件，并定时扫描补偿；云端校验成功后才清理本地缓存。":"工作引擎未启用，当前不会自动传输或删除文件。"}</p></div></div></Card></div>
 }
 const rows=q.data as Row[], fields=columns[page]
 return <div className="space-y-5"><AdminControls page={page} token={token} rows={rows}/><Card className="overflow-hidden"><div className="flex items-center justify-between border-b border-border p-5"><h2 className="font-medium">{pages.find(p=>p.key===page)?.label}</h2><span className="text-xs text-muted-foreground">最近 {rows.length} 条</span></div><div className="hidden overflow-x-auto md:block"><table className="w-full whitespace-nowrap text-left text-sm"><thead className="border-b border-border text-muted-foreground"><tr>{fields.map(([key,label])=><th className="px-5 py-3 font-medium" key={key}>{label}</th>)}</tr></thead><tbody>{rows.map(row=><tr className="border-b border-border last:border-0" key={String(row.id)}>{fields.map(([key])=><td className="max-w-80 truncate px-5 py-4" key={key}>{value(key,row[key])}</td>)}</tr>)}</tbody></table></div><div className="divide-y divide-border md:hidden" data-testid="mobile-records">{rows.map(row=><article key={String(row.id)} className="min-w-0 space-y-3 p-4"><h3 className="break-all text-sm font-semibold">{value(fields[0][0],row[fields[0][0]])}</h3><dl className="grid grid-cols-2 gap-x-3 gap-y-3">{fields.slice(1).map(([key,label])=><div key={key} className={cn("min-w-0",key==="last_error"&&"col-span-2")}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 break-all text-sm">{value(key,row[key])}</dd></div>)}</dl></article>)}</div>{rows.length===0&&<div className="flex flex-col items-center gap-2 py-16 text-muted-foreground"><Archive className="size-8 opacity-50"/><p className="text-sm">暂无记录</p><p className="text-xs">请先添加账号、授权并配置备份策略。</p></div>}</Card></div>
}
function App(){
 const [page,setPage]=useState<Page>("overview"),[token,setToken]=useState(""),[draft,setDraft]=useState(""),[menuOpen,setMenuOpen]=useState(false)
 const [dark,setDark]=useState(()=>window.matchMedia("(prefers-color-scheme: dark)").matches)
 const qc=useQueryClient()
 useEffect(()=>{document.documentElement.classList.toggle("dark",dark)},[dark])
 const theme=<button aria-label="切换主题" className="icon-button rounded-lg border border-border bg-card" onClick={()=>setDark(!dark)}>{dark?<Sun className="size-4"/>:<Moon className="size-4"/>}</button>
 if(!token)return <main className="login-shell grid min-h-dvh place-items-center p-4"><div className="login-theme absolute">{theme}</div><Card className="w-full min-w-0 max-w-sm p-5 sm:p-8"><div className="mb-6 flex size-11 items-center justify-center rounded-xl bg-blue-600 text-white"><Send className="size-5"/></div><h1 className="text-2xl font-semibold tracking-tight">TelegramBackup</h1><p className="mt-2 text-sm text-muted-foreground">登录自托管备份控制台</p><form className="mt-6 space-y-3" onSubmit={e=>{e.preventDefault();qc.clear();setToken(draft.trim());setDraft("")}}><label htmlFor="token" className="text-sm font-medium">管理令牌</label><input id="token" type="password" autoComplete="off" required minLength={32} className="w-full rounded-lg border border-border bg-background px-3 py-3 text-base md:text-sm" value={draft} onChange={e=>setDraft(e.target.value)} placeholder="输入 TGB_ADMIN_TOKEN"/><button type="submit" className="min-h-11 w-full rounded-lg bg-blue-600 py-2.5 text-sm font-medium text-white hover:bg-blue-700">进入控制台</button></form><p className="mt-4 text-xs text-muted-foreground">令牌仅保存在当前页面内存中。请通过可信 HTTPS 或本地连接访问。</p></Card></main>
 return <div className="app-shell min-h-dvh md:grid md:grid-cols-[230px_minmax(0,1fr)]">
 <aside className="min-w-0 border-b border-border bg-card md:sticky md:top-0 md:h-dvh md:overflow-y-auto md:border-b-0 md:border-r">
  <div className="flex min-w-0 items-center gap-3 px-4 py-3 md:p-5">
   <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-blue-600 text-white"><Send className="size-4"/></span>
   <div className="min-w-0 flex-1"><p className="break-words text-sm font-semibold tracking-tight">TelegramBackup</p><p className="text-xs text-muted-foreground">自托管控制台</p></div>
   <button type="button" aria-label={menuOpen?"关闭导航":"打开导航"} aria-expanded={menuOpen} aria-controls="main-navigation" className="icon-button rounded-lg border border-border md:hidden" onClick={()=>setMenuOpen(v=>!v)}>{menuOpen?<X className="size-5"/>:<Menu className="size-5"/>}</button>
  </div>
  <nav id="main-navigation" className={cn("flex-col gap-1 px-3 pb-3 md:flex md:pt-4",menuOpen?"flex":"hidden")} aria-label="主导航">
   {pages.map(p=><button key={p.key} aria-current={page===p.key?"page":undefined} onClick={()=>{setPage(p.key);setMenuOpen(false)}} className={cn("flex min-h-11 items-center gap-2.5 rounded-lg px-3 py-2.5 text-left text-sm text-muted-foreground hover:bg-background",page===p.key&&"bg-background font-medium text-foreground ring-1 ring-border")}><p.icon className="size-4 shrink-0"/>{p.label}</button>)}
  </nav>
 </aside>
 <main className="min-w-0">
  <header className="flex min-w-0 flex-wrap items-center justify-between gap-2 border-b border-border bg-card px-3 py-3 sm:px-5 md:px-8">
   <div className="min-w-0 flex-1 break-words text-sm text-muted-foreground"><span className="hidden sm:inline">工作空间 / </span><span className="text-foreground">{pages.find(p=>p.key===page)?.label}</span></div>
   <div className="flex shrink-0 gap-1.5 sm:gap-2">{theme}<button aria-label="刷新数据" className="icon-button rounded-lg border border-border" onClick={()=>void qc.invalidateQueries()}><RefreshCw className="size-4"/></button><button aria-label="退出" className="icon-button rounded-lg border border-border" onClick={()=>{setToken("");setPage("overview");setMenuOpen(false);qc.clear()}}><LogOut className="size-4"/></button></div>
  </header>
  <div className="mx-auto min-w-0 max-w-7xl space-y-4 p-3 sm:p-5 md:p-8"><div><h1 className="text-xl font-semibold tracking-tight sm:text-2xl">{pages.find(p=>p.key===page)?.label}</h1><p className="mt-1 text-sm text-muted-foreground">Telegram 媒体备份与归档管理</p></div><DataView page={page} token={token}/></div>
 </main></div>
}
createRoot(document.getElementById("root")!).render(<React.StrictMode><QueryClientProvider client={client}><App/></QueryClientProvider></React.StrictMode>)
