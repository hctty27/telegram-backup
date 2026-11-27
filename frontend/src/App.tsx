import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  Form,
  Input,
  Layout,
  Menu,
  Progress,
  Row,
  Space,
  Statistic,
  Switch,
  Table,
  Tag,
  Typography,
  message
} from "antd";
import {
  CloudDownloadOutlined,
  CloudUploadOutlined,
  DashboardOutlined,
  FolderOpenOutlined,
  LockOutlined,
  ReloadOutlined,
  SettingOutlined,
  SyncOutlined,
  ThunderboltOutlined
} from "@ant-design/icons";
import { createApi } from "./api";
import SettingsPage from "./SettingsPage";
import { ArchivePage, MonitorPanel } from "./OperationsPage";
import type { AppSettings, BackupTask, Chat, DashboardStats } from "./types";

const { Header, Sider, Content } = Layout;
const { Title, Text, Paragraph } = Typography;

function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, index)).toFixed(index === 0 ? 0 : 2)} ${units[index]}`;
}

function formatDate(value: string | null): string {
  return value ? new Date(value).toLocaleString() : "从未备份";
}

function statusTag(status: string) {
  const color = status === "completed" ? "success"
    : status === "running" ? "processing"
    : status === "failed" ? "error"
    : "default";
  const label = status === "completed" ? "已完成"
    : status === "running" ? "运行中"
    : status === "failed" ? "失败"
    : "排队中";
  return <Tag color={color}>{label}</Tag>;
}

export default function App() {
  const [token, setToken] = useState(localStorage.getItem("telegram-backup-token") || "");
  const [authenticated, setAuthenticated] = useState(false);
  const [page, setPage] = useState("overview");
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [chats, setChats] = useState<Chat[]>([]);
  const [tasks, setTasks] = useState<BackupTask[]>([]);
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [loading, setLoading] = useState(false);
  const [working, setWorking] = useState(false);
  const [loginError, setLoginError] = useState("");

  const api = useMemo(() => createApi(token), [token]);

  const loadData = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    try {
      const [dashboard, chatRows, taskRows, appSettings] = await Promise.all([
        api.dashboard(), api.chats(), api.tasks(), api.settings()
      ]);
      setStats(dashboard);
      setChats(chatRows);
      setTasks(taskRows);
      setSettings(appSettings);
    } finally {
      setLoading(false);
    }
  }, [api, token]);

  useEffect(() => {
    if (!token) {
      setAuthenticated(false);
      return;
    }
    api.settings()
      .then(() => {
        setAuthenticated(true);
        setLoginError("");
        void loadData();
      })
      .catch(() => {
        setAuthenticated(false);
        localStorage.removeItem("telegram-backup-token");
      });
  }, [api, loadData, token]);

  const runAction = async (action: () => Promise<unknown>, successText: string) => {
    setWorking(true);
    try {
      await action();
      message.success(successText);
      await loadData();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "操作失败");
    } finally {
      setWorking(false);
    }
  };

  const handleLogin = async (values: { token: string }) => {
    const candidate = values.token.trim();
    setWorking(true);
    setLoginError("");
    try {
      await createApi(candidate).settings();
      localStorage.setItem("telegram-backup-token", candidate);
      setToken(candidate);
      setAuthenticated(true);
      message.success("登录成功");
    } catch (error) {
      setLoginError(error instanceof Error ? error.message : "Token 验证失败");
    } finally {
      setWorking(false);
    }
  };

  const logout = () => {
    localStorage.removeItem("telegram-backup-token");
    setToken("");
    setAuthenticated(false);
    setStats(null);
    setPage("overview");
    setChats([]);
    setTasks([]);
  };

  if (!authenticated) {
    return (
      <div className="login-shell">
        <Card className="login-card" bordered={false}>
          <div className="brand-mark"><CloudDownloadOutlined /></div>
          <Title level={2}>TelegramBackup</Title>
          <Paragraph type="secondary">登录你的私有备份管理控制台</Paragraph>
          <Form layout="vertical" onFinish={handleLogin}>
            <Form.Item
              label="管理员 Token"
              name="token"
              rules={[{ required: true, message: "请输入 .env 中配置的 ADMIN_TOKEN" }]}
            >
              <Input.Password
                size="large"
                prefix={<LockOutlined />}
                placeholder="ADMIN_TOKEN"
              />
            </Form.Item>
            {loginError && <Alert type="error" showIcon message={loginError} className="login-error" />}
            <Button type="primary" htmlType="submit" size="large" block loading={working}>
              进入管理后台
            </Button>
          </Form>
          <Text type="secondary" className="login-footnote">Token 仅保存在当前浏览器本地</Text>
        </Card>
      </div>
    );
  }

  const chatColumns = [
    {
      title: "群组 / 频道",
      dataIndex: "title",
      key: "title",
      render: (_: string, chat: Chat) => (
        <Space direction="vertical" size={0}>
          <Text strong>{chat.title}</Text>
          <Text type="secondary" className="table-subtitle">
            {chat.username ? `@${chat.username}` : chat.telegram_id}
          </Text>
        </Space>
      )
    },
    { title: "类型", dataIndex: "chat_type", key: "chat_type", render: (v: string) => <Tag>{v}</Tag> },
    {
      title: "备份",
      dataIndex: "backup_enabled",
      key: "backup_enabled",
      render: (enabled: boolean, chat: Chat) => (
        <Switch
          checked={enabled}
          onChange={(value) => void runAction(
            () => api.updateChat(chat.id, value),
            value ? "已开启自动备份" : "已暂停自动备份"
          )}
        />
      )
    },
    { title: "最后同步", dataIndex: "last_synced_at", key: "last_synced_at", render: formatDate },
    {
      title: "操作",
      key: "actions",
      render: (_: unknown, chat: Chat) => (
        <Button
          size="small"
          icon={<CloudUploadOutlined />}
          loading={working}
          onClick={() => void runAction(() => api.startBackup(chat.id), "备份任务已加入队列")}
        >
          立即备份
        </Button>
      )
    }
  ];

  const taskColumns = [
    { title: "任务", dataIndex: "id", key: "id", render: (id: number) => <Text code>#{id}</Text> },
    { title: "群组 ID", dataIndex: "chat_id", key: "chat_id" },
    { title: "状态", dataIndex: "status", key: "status", render: statusTag },
    { title: "扫描消息", dataIndex: "scanned_messages", key: "scanned_messages" },
    { title: "下载文件", dataIndex: "downloaded_files", key: "downloaded_files" },
    { title: "上传文件", dataIndex: "uploaded_files", key: "uploaded_files" },
    { title: "创建时间", dataIndex: "created_at", key: "created_at", render: formatDate },
    {
      title: "操作",
      key: "actions",
      render: (_: unknown, task: BackupTask) => task.status === "failed"
        ? <Button size="small" onClick={() => void runAction(() => api.retryTask(task.id), "任务已重新排队")}>重新执行</Button>
        : <Text type="secondary">—</Text>
    }
  ];

  const menuItems = [
    { key: "overview", icon: <DashboardOutlined />, label: "总览" },
    { key: "chats", icon: <FolderOpenOutlined />, label: "群组管理" },
    { key: "tasks", icon: <ThunderboltOutlined />, label: "备份任务" },
    { key: "archive", icon: <FolderOpenOutlined />, label: "文件归档" },
    { key: "monitor", icon: <DashboardOutlined />, label: "运行日志" },
    { key: "settings", icon: <SettingOutlined />, label: "系统设置" }
  ];

  return (
    <Layout className="app-shell">
      <Sider width={232} breakpoint="lg" collapsedWidth="0" className="app-sider">
        <div className="sidebar-brand">
          <div className="brand-mark small"><CloudDownloadOutlined /></div>
          <div>
            <div className="brand-title">TelegramBackup</div>
            <div className="brand-caption">PERSONAL ARCHIVE</div>
          </div>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[page]}
          items={menuItems}
          onClick={({ key }) => setPage(key)}
          className="side-menu"
        />
        <div className="sidebar-bottom">
          <Badge status={settings?.telegram_authorized ? "success" : "warning"} text="Telegram 账号" />
          <div className="sidebar-version">v0.1.0 · Self-hosted</div>
        </div>
      </Sider>

      <Layout>
        <Header className="topbar">
          <div>
            <Text type="secondary">工作空间</Text>
            <div className="topbar-title">
              {menuItems.find((item) => item.key === page)?.label || "总览"}
            </div>
          </div>
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading}>刷新</Button>
            <Button type="text" onClick={logout}>退出</Button>
          </Space>
        </Header>

        <div className="mobile-nav"><Menu mode="horizontal" selectedKeys={[page]} items={menuItems} onClick={({key})=>setPage(key)} /></div>
        <Content className="page-content">
          {loading && !stats ? <div className="page-loading"><Progress percent={60} status="active" showInfo={false} /></div> : null}

          {page === "overview" && (
            <Space direction="vertical" size={20} className="full-width">
              <div className="page-heading">
                <div>
                  <Title level={3}>备份总览</Title>
                  <Text type="secondary">查看 Telegram 媒体归档运行情况</Text>
                </div>
                <Button type="primary" icon={<SyncOutlined />} loading={working} onClick={() => void runAction(() => api.syncChats(), "Telegram 群组同步完成")}>
                  同步 Telegram 群组
                </Button>
              </div>
              {!settings?.telegram_authorized && (
                <Alert
                  type="warning"
                  showIcon
                  message="Telegram 尚未授权"
                  description="请进入「系统设置 → Telegram 账号」，填写 API ID / API Hash，然后通过手机号验证码完成登录。"
                />
              )}
              {!settings?.rclone_configured && (
                <Alert
                  type="info"
                  showIcon
                  message="尚未配置 rclone"
                  description="请进入「系统设置 → 云存储」，添加 Google Drive 或选择已有 rclone Remote，并测试连接。"
                />
              )}
              <MonitorPanel api={api}/>
              <Row gutter={[16, 16]}>
                <Col xs={24} sm={12} xl={6}><Card className="metric-card"><Statistic title="已导入群组" value={stats?.total_chats || 0} suffix="个" /></Card></Col>
                <Col xs={24} sm={12} xl={6}><Card className="metric-card"><Statistic title="自动备份中" value={stats?.enabled_chats || 0} suffix="个" /></Card></Col>
                <Col xs={24} sm={12} xl={6}><Card className="metric-card"><Statistic title="已上传文件" value={stats?.uploaded_files || 0} suffix="个" /></Card></Col>
                <Col xs={24} sm={12} xl={6}><Card className="metric-card"><Statistic title="归档容量" value={formatBytes(stats?.total_bytes || 0)} /></Card></Col>
              </Row>
              <Row gutter={[16, 16]}>
                <Col xs={24} lg={8}><Card title="任务队列" className="panel-card"><div className="queue-stats"><div><Text type="secondary">排队中</Text><Title level={2}>{stats?.queued_tasks || 0}</Title></div><div><Text type="secondary">运行中</Text><Title level={2} className="accent-number">{stats?.running_tasks || 0}</Title></div><div><Text type="secondary">失败</Text><Title level={2} className="danger-number">{stats?.failed_tasks || 0}</Title></div></div></Card></Col>
                <Col xs={24} lg={16}><Card title="最近任务" extra={<Button type="link" onClick={() => setPage("tasks")}>查看全部</Button>} className="panel-card">
                  {tasks.length ? <Table rowKey="id" size="small" pagination={false} dataSource={tasks.slice(0, 5)} scroll={{x:600}} columns={taskColumns.slice(0, 7)} /> : <Empty description="还没有备份任务" />}
                </Card></Col>
              </Row>
            </Space>
          )}

          {page === "chats" && (
            <Card className="panel-card" title="Telegram 群组与频道" extra={
              <Button type="primary" icon={<SyncOutlined />} loading={working} onClick={() => void runAction(() => api.syncChats(), "群组同步完成")}>从 Telegram 同步</Button>
            }>
              <Paragraph type="secondary">选择需要归档的群组。开启自动备份后，后台会按设定间隔扫描新消息。</Paragraph>
              <Table rowKey="id" dataSource={chats} scroll={{x:650}} columns={chatColumns} loading={loading} pagination={{ pageSize: 10, showSizeChanger: false }} locale={{ emptyText: <Empty description="尚无群组，请先同步 Telegram" /> }} />
            </Card>
          )}

          {page === "tasks" && (
            <Card className="panel-card" title="备份任务">
              <Paragraph type="secondary">查看备份执行状态、扫描数量和上传结果。失败任务可以重新执行。</Paragraph>
              <Table rowKey="id" dataSource={tasks} scroll={{x:900}} columns={taskColumns} loading={loading} pagination={{ pageSize: 10, showSizeChanger: false }} locale={{ emptyText: <Empty description="暂无任务" /> }} />
            </Card>
          )}

          {page === "archive" && <ArchivePage api={api} chats={chats}/>}
          {page === "monitor" && <MonitorPanel api={api} logs/>}
          {page === "settings" && <SettingsPage api={api} onRefresh={loadData} />}

        </Content>
      </Layout>
    </Layout>
  );
}
