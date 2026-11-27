import {useEffect,useState} from "react"
import {Activity,HardDrive} from "lucide-react"
import {Card} from "@/components/ui/card"

type Disk={staged_bytes:number;free_bytes:number;quota_bytes:number;reserve_bytes:number;promised_bytes:number;available_bytes:number}
type Transfer={id:string;media_id:string;stage:string;bytes:number;total:number;speed:number}
type Snapshot={disk:Disk|null;transfers:Transfer[]}
function size(n:number){
 if(!Number.isFinite(n)||n<0)return "—"
 if(n===0)return "0 B"
 const u=["B","KiB","MiB","GiB","TiB"],i=Math.min(4,Math.max(0,Math.floor(Math.log(n)/Math.log(1024))))
 return (n/1024**i).toFixed(i?1:0)+" "+u[i]
}
function useMonitor(token:string,enabled:boolean){
 const [snapshot,setSnapshot]=useState<Snapshot|null>(null)
 const [connected,setConnected]=useState(false)
 useEffect(()=>{
  if(!enabled)return
  let stopped=false
  let controller:AbortController|undefined
  let retry:ReturnType<typeof setTimeout>|undefined
  let failures=0
  async function connect(){
   controller=new AbortController()
   let reader:ReadableStreamDefaultReader<Uint8Array>|undefined
   try{
    const res=await fetch("/api/v2/monitor/events",{headers:{Authorization:"Bearer "+token,Accept:"text/event-stream"},signal:controller.signal,cache:"no-store"})
    if(!res.ok||!res.body)throw new Error("stream unavailable")
    reader=res.body.getReader()
    const decoder=new TextDecoder()
    let pending=""
    while(!stopped){
     const {done,value}=await reader.read()
     if(done)break
     pending+=decoder.decode(value,{stream:true})
     if(pending.length>256*1024)throw new Error("stream frame too large")
     let end:number
     while((end=pending.indexOf("\n\n"))!==-1){
      const frame=pending.slice(0,end);pending=pending.slice(end+2)
      const line=frame.split("\n").find(s=>s.startsWith("data: "))
      if(!line)continue
      const next=JSON.parse(line.slice(6)) as Snapshot
      if(!Array.isArray(next.transfers))throw new Error("invalid snapshot")
      if(!stopped){setSnapshot(next);setConnected(true);failures=0}
     }
    }
   }catch{ /* network errors are handled by reconnect; never log credentials */ }
   finally{
    if(reader){try{await reader.cancel()}catch{}reader.releaseLock()}
    if(!stopped){
     setConnected(false)
     const delay=Math.min(15000,1000*2**Math.min(failures++,4))
     retry=setTimeout(()=>void connect(),delay)
    }
   }
  }
  setSnapshot(null);setConnected(false);void connect()
  return ()=>{stopped=true;controller?.abort();if(retry)clearTimeout(retry)}
 },[token,enabled])
 return {snapshot,connected}
}
function Bar({n,total,label}:{n:number;total:number;label:string}){
 const percent=total>0?Math.max(0,Math.min(100,n/total*100)):0
 return <div className="h-2 overflow-hidden rounded-full bg-background ring-1 ring-border" role="progressbar" aria-label={label} aria-valuenow={Math.round(percent)} aria-valuemin={0} aria-valuemax={100}><div className="h-full rounded-full bg-blue-600 transition-[width]" style={{width:percent+"%"}}/></div>
}
export function LiveMonitor({token,enabled}:{token:string;enabled:boolean}){
 const {snapshot,connected}=useMonitor(token,enabled)
 if(!enabled)return null
 const disk=snapshot?.disk
 return <div className="grid gap-4 lg:grid-cols-2">
 <Card className="space-y-4 p-5"><div className="flex items-center justify-between"><h2 className="flex items-center gap-2 font-medium"><HardDrive className="size-4"/>磁盘保护</h2><span className="text-xs text-muted-foreground">{connected?"实时":"等待连接"}</span></div>
 {disk?<><p className="text-2xl font-semibold tabular-nums">{size(disk.staged_bytes)} <span className="text-sm font-normal text-muted-foreground">/ {size(disk.quota_bytes)}</span></p><Bar n={disk.staged_bytes} total={disk.quota_bytes} label="暂存空间"/><dl className="grid grid-cols-2 gap-3 text-sm">{[["磁盘剩余",disk.free_bytes],["系统保留",disk.reserve_bytes],["任务预留",disk.promised_bytes],["可新增下载",disk.available_bytes]].map(([label,n])=><div key={String(label)}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 tabular-nums">{size(Number(n))}</dd></div>)}</dl>{disk.available_bytes===0&&<p className="text-sm text-amber-600">磁盘空间不足，暂停接纳新的下载；上传和清理仍可继续。</p>}</>:<p className="text-sm text-muted-foreground">等待磁盘测量数据。</p>}</Card>
 <Card className="space-y-4 p-5"><div className="flex items-center justify-between"><h2 className="flex items-center gap-2 font-medium"><Activity className="size-4"/>实时传输</h2><span className="text-xs text-muted-foreground">{connected?"已连接":"重连中"}</span></div>
 {!snapshot?.transfers.length?<p className="py-6 text-center text-sm text-muted-foreground">{connected?"暂无活跃传输":"等待监控数据"}</p>:snapshot.transfers.map(t=><div key={t.id} className="space-y-2 border-t border-border pt-3"><div className="flex justify-between gap-2 text-sm"><span>{t.stage==="download"?"下载":"上传 / 校验"} · #{t.media_id}</span><span className="tabular-nums">{connected?size(t.speed)+"/s":"—"}</span></div><Bar n={t.bytes} total={t.total} label={"媒体 "+t.media_id}/><p className="text-xs text-muted-foreground">{size(t.bytes)} / {size(t.total)} · {t.total>0?Math.round(t.bytes/t.total*100):0}%</p></div>)}
 {!connected&&snapshot&&<p className="text-xs text-amber-600">连接已断开，显示最后一次快照。</p>}</Card>
 </div>
}
