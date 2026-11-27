import { useEffect, useState } from "react";
import { Alert, Card, Empty, Input, Progress, Select, Space, Table, Tag, Typography } from "antd";
import type { createApi } from "./api";
import type { Chat } from "./types";

type Api = ReturnType<typeof createApi>;
type Active = {task_id:number; file_name:string; phase:string; bytes:number; total_bytes:number; speed:number};
type Event = {time:string; task_id:number; action:string; file_name:string|null};
export type MonitorData = {active:Active|null; events:Event[]};
type ArchiveFile = {id:number; chat_title:string; file_name:string; mime_type:string|null; file_size:number; remote_path:string|null; uploaded_at:string|null};
export type ArchiveData = {total:number;items:ArchiveFile[]};
const labels: Record<string,string> = {downloading:"下载中", uploading:"上传中", task_started:"任务开始", file_uploaded:"文件上传完成", task_completed:"任务完成", task_failed:"任务失败"};
function size(n:number) { if (!Number.isFinite(n) || n <= 0) return "0 B"; const i=Math.max(0,Math.min(4,Math.floor(Math.log(n)/Math.log(1024)))); return (n/1024**i).toFixed(i?1:0)+" "+["B","KiB","MiB","GiB","TiB"][i]; }

export function MonitorPanel({api, logs=false}:{api:Api;logs?:boolean}) {
  const [data,setData]=useState<MonitorData|null>(null);
  const [error,setError]=useState("");
  useEffect(()=>{
    let closed=false, busy=false;
    const load=async()=>{if(busy)return;busy=true;try{const next=await api.monitor();if(!closed){setData(next);setError("");}}catch{if(!closed)setError("无法获取运行状态");}finally{busy=false;}};
    void load(); const id=window.setInterval(load,2000);
    return ()=>{closed=true;window.clearInterval(id);};
  },[api]);
  const active=data?.active;
  const pct=active?.total_bytes?Math.min(100,Math.round(active.bytes/active.total_bytes*100)):0;
  return <Space direction="vertical" className="full-width" size={16}>
    {error&&<Alert type="warning" message={error} />}
    <Card className="panel-card" title="实时传输">
      {active?<Space direction="vertical" className="full-width">
        <Tag color="processing">{labels[active.phase]||active.phase} · 任务 #{active.task_id}</Tag>
        <Typography.Text style={{overflowWrap:"anywhere"}}>{active.file_name}</Typography.Text>
        {active.total_bytes>0?<Progress percent={pct} status="active"/>:<Typography.Text type="secondary">正在计算文件大小</Typography.Text>}
        <Typography.Text>{size(active.bytes)} / {active.total_bytes?size(active.total_bytes):"未知"} · {size(active.speed)}/s</Typography.Text>
      </Space>:<Empty description={data?"当前没有文件传输":"正在加载运行状态"} image={Empty.PRESENTED_IMAGE_SIMPLE}/>}
      <Typography.Text type="secondary">每 2 秒刷新；显示当前单个文件，不代表整个群组的完成率。</Typography.Text>
    </Card>
    {logs&&<Card className="panel-card" title="本次运行日志">
      <Typography.Paragraph type="secondary">最近 200 条结构化备份事件，服务重启后清空。历史任务结果仍在备份任务页面。</Typography.Paragraph>
      <Table rowKey={(_,i)=>String(i)} scroll={{x:560}} pagination={{pageSize:15}} dataSource={data?.events||[]} columns={[
        {title:"时间",dataIndex:"time",render:(v:string)=>new Date(v).toLocaleString()},
        {title:"任务",dataIndex:"task_id"},
        {title:"事件",dataIndex:"action",render:(v:string)=><Tag color={v==="task_failed"?"error":"default"}>{labels[v]||v}</Tag>},
        {title:"文件",dataIndex:"file_name",render:(v:string|null)=>v||"—"}
      ]}/>
    </Card>}
  </Space>;
}

export function ArchivePage({api,chats}:{api:Api;chats:Chat[]}) {
  const [q,setQ]=useState(""); const [kind,setKind]=useState("all");const [chat,setChat]=useState<number|undefined>();
  const [page,setPage]=useState(1);const [data,setData]=useState<ArchiveData>({total:0,items:[]});const [busy,setBusy]=useState(false);const [error,setError]=useState("");
  useEffect(()=>{
    let closed=false; setBusy(true);
    const params=new URLSearchParams({q,kind,page:String(page),page_size:"30"});
    if(chat!==undefined)params.set("chat_id",String(chat));
    api.archive(params).then(v=>{if(!closed){setData(v);setError("");}}).catch(()=>{if(!closed)setError("读取归档记录失败");}).finally(()=>{if(!closed)setBusy(false);});
    return ()=>{closed=true;};
  },[api,q,kind,chat,page]);
  return <Card className="panel-card" title="文件归档">
    <Typography.Paragraph type="secondary">显示数据库中已成功上传的文件记录。不会删除云盘文件，也不保证远端文件仍然存在。</Typography.Paragraph>
    <Space wrap style={{marginBottom:16}}>
      <Input.Search placeholder="搜索文件名" allowClear onSearch={v=>{setQ(v);setPage(1);}}/>
      <Select value={kind} style={{width:120}} onChange={v=>{setKind(v);setPage(1);}} options={[{value:"all",label:"全部类型"},{value:"image",label:"图片"},{value:"video",label:"视频"},{value:"other",label:"其他"}]}/>
      <Select allowClear showSearch optionFilterProp="label" placeholder="全部群组" style={{minWidth:160,maxWidth:280}} value={chat} onChange={v=>{setChat(v);setPage(1);}} options={chats.map(c=>({value:c.id,label:c.title}))}/>
    </Space>
    {error&&<Alert type="error" message={error}/>}
    <Table rowKey="id" loading={busy} scroll={{x:800}} dataSource={data.items} pagination={{current:page,total:data.total,pageSize:30,showSizeChanger:false,onChange:setPage}} columns={[
      {title:"文件名称",dataIndex:"file_name",render:(v:string)=><Typography.Text style={{overflowWrap:"anywhere"}}>{v}</Typography.Text>},
      {title:"来源群组",dataIndex:"chat_title"},
      {title:"类型",dataIndex:"mime_type",render:(v:string|null)=>v||"—"},
      {title:"大小",dataIndex:"file_size",render:size},
      {title:"上传时间",dataIndex:"uploaded_at",render:(v:string|null)=>v?new Date(v.endsWith("Z")||/[+-]\d\d:\d\d$/.test(v)?v:v+"Z").toLocaleString():"—"},
      {title:"云端路径",dataIndex:"remote_path",render:(v:string|null)=>v?<Typography.Text copyable={{text:v}}>复制路径</Typography.Text>:"—"}
    ]}/>
  </Card>;
}
