import { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Divider,
  Input,
  InputNumber,
  Select,
  Space,
  Tag,
  Typography,
  message
} from "antd";
import {
  CloudOutlined,
  LockOutlined,
  SaveOutlined,
  SendOutlined,
  SettingOutlined,
  CheckCircleOutlined
} from "@ant-design/icons";
import { createApi } from "./api";
import type { AppSettings } from "./types";

const { Text, Paragraph, Title } = Typography;
const { TextArea, Password } = Input;

type Api = ReturnType<typeof createApi>;

interface Props {
  api: Api;
  onRefresh: () => Promise<void>;
}

export default function SettingsPage({ api, onRefresh }: Props) {
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [busy, setBusy] = useState(false);

  const [apiId, setApiId] = useState<number | null>(null);
  const [apiHash, setApiHash] = useState("");
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [codeSent, setCodeSent] = useState(false);
  const [codeDelivery, setCodeDelivery] = useState<{
    delivery_type: string; delivery_label: string; next_type: string | null;
    next_label: string | null; timeout: number | null;
    email_pattern?: string | null; fragment_url?: string | null;
  } | null>(null);
  const [needsPassword, setNeedsPassword] = useState(false);

  const [remoteName, setRemoteName] = useState("google-drive");
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [tokenJson, setTokenJson] = useState("");
  const [rootFolderId, setRootFolderId] = useState("");
  const [destination, setDestination] = useState("");
  const [scanInterval, setScanInterval] = useState(300);

  const refresh = async () => {
    const value = await api.settings();
    setSettings(value);
    setApiId(value.telegram_api_id);
    setDestination(value.rclone_remote || "");
    setScanInterval(value.scan_interval_seconds || 300);
  };

  useEffect(() => {
    void refresh().catch((error) => message.error(error instanceof Error ? error.message : "读取系统配置失败"));
  }, []);

  const run = async (action: () => Promise<unknown>, success: string) => {
    setBusy(true);
    try {
      await action();
      message.success(success);
      await refresh();
      await onRefresh();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "操作失败");
    } finally {
      setBusy(false);
    }
  };

  const saveTelegram = () => {
    if (!apiId) {
      message.warning("请输入 Telegram API ID");
      return;
    }
    if (!settings?.encryption_ready) {
      message.error("请先在服务器配置 APP_ENCRYPTION_KEY，再保存敏感凭据");
      return;
    }
    void run(
      () => api.saveTelegram({ api_id: apiId, ...(apiHash.trim() ? { api_hash: apiHash.trim() } : {}) }),
      "Telegram API 配置已保存"
    );
    setApiHash("");
  };

  const sendCode = () => {
    if (!phone.trim()) {
      message.warning("请输入带国家区号的手机号，例如 +8613800000000");
      return;
    }
    void run(async () => {
      const result = await api.sendTelegramCode(phone.trim());
      setCodeDelivery(result);
      setCodeSent(true);
      setNeedsPassword(false);
    }, "Telegram 已接受验证码请求，请按下方显示的渠道查收");
  };

  const verifyCode = () => {
    if (!code.trim()) {
      message.warning("请输入 Telegram 验证码");
      return;
    }
    void run(async () => {
      const result = await api.verifyTelegramCode(code.trim());
      setNeedsPassword(result.needs_password);
      if (result.authorized) setCodeSent(false);
    }, "验证码已验证");
  };

  const verifyPassword = () => {
    if (!password) {
      message.warning("请输入 Telegram 两步验证密码");
      return;
    }
    void run(async () => {
      await api.verifyTelegramPassword(password);
      setNeedsPassword(false);
      setCodeSent(false);
      setPassword("");
    }, "Telegram 账号授权成功");
  };

  const addGoogleDrive = () => {
    if (!settings?.encryption_ready) {
      message.error("请先在服务器配置 APP_ENCRYPTION_KEY");
      return;
    }
    void run(async () => {
      const result = await api.addGoogleDrive({
        name: remoteName.trim(),
        client_id: clientId.trim(),
        client_secret: clientSecret.trim(),
        token_json: tokenJson.trim(),
        root_folder_id: rootFolderId.trim()
      });
      setDestination(result.destination);
      setClientSecret("");
      setTokenJson("");
    }, "Google Drive 已添加并完成连接测试");
  };

  const testStorage = () => {
    void run(() => api.testStorage(destination.trim() || undefined), "云存储连接测试成功");
  };

  const saveDestination = () => {
    if (!destination.trim()) {
      message.warning("请输入备份目标，例如 backup:TelegramBackup");
      return;
    }
    void run(() => api.setStorageDestination(destination.trim()), "默认备份目标已保存");
  };

  const saveBackup = () => {
    if (!scanInterval || scanInterval < 60 || scanInterval > 86400) {
      message.warning("扫描间隔必须在 60 到 86400 秒之间");
      return;
    }
    void run(() => api.saveBackupSettings(scanInterval), "备份策略已保存");
  };

  return (
    <Space direction="vertical" size={16} className="full-width">
      <div className="page-heading">
        <div>
          <Title level={3}>系统设置</Title>
          <Text type="secondary">配置 Telegram 账号、云存储和自动备份策略</Text>
        </div>
        <Button onClick={() => void refresh()} loading={busy}>刷新状态</Button>
      </div>

      {!settings?.encryption_ready && (
        <Alert
          type="warning"
          showIcon
          message="尚未配置应用加密密钥"
          description={
            <span>
              保存 Telegram API Hash、Google OAuth 凭据前，需要在服务器的 .env 中配置 APP_ENCRYPTION_KEY。
              生成命令：<Text code>python -c "from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())"</Text>。
              设置后重启 backend。不要丢失或更换该密钥。
            </span>
          }
        />
      )}

      <Card className="panel-card" title={<Space><SendOutlined />Telegram 账号</Space>}>
        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label="API 配置">
            {settings?.telegram_configured ? <Tag color="success">已配置</Tag> : <Tag color="warning">未配置</Tag>}
          </Descriptions.Item>
          <Descriptions.Item label="账号授权">
            {settings?.telegram_authorized ? <Tag color="success">已登录</Tag> : <Tag color="default">未登录</Tag>}
          </Descriptions.Item>
        </Descriptions>
        <Divider />
        <Title level={5}>Telegram API 凭据</Title>
        <Paragraph type="secondary">在 my.telegram.org 获取 API ID 和 API Hash。API Hash 只写入加密存储，不会回显到浏览器。</Paragraph>
        <Space direction="vertical" className="full-width">
          <InputNumber
            min={1}
            value={apiId ?? undefined}
            onChange={(value) => setApiId(value)}
            placeholder="API ID"
            style={{ width: "100%" }}
          />
          <Password
            value={apiHash}
            onChange={(event) => setApiHash(event.target.value)}
            placeholder={settings?.telegram_configured ? "API Hash 已保存；留空表示不修改" : "API Hash"}
          />
          <Button type="primary" icon={<SaveOutlined />} loading={busy} onClick={saveTelegram} disabled={!settings?.encryption_ready}>
            保存 Telegram API 配置
          </Button>
        </Space>
        <Divider />
        <Title level={5}>用户账号登录</Title>
        <Paragraph type="secondary">填写手机号并接收 Telegram 登录验证码；如果账号启用了两步验证，随后还需要输入 2FA 密码。</Paragraph>
        <Space direction="vertical" className="full-width">
          <Input value={phone} onChange={(event) => setPhone(event.target.value)} placeholder="手机号，例如 +8613800000000" />
          <Button onClick={sendCode} loading={busy} disabled={!settings?.telegram_configured}>
            发送登录验证码
          </Button>
          {codeSent && codeDelivery && (
            <Alert
              type="info"
              showIcon
              message={`Telegram 返回的投递方式：${codeDelivery.delivery_label}`}
              description={
                <Space direction="vertical" size={4}>
                  <Text>返回类型：{codeDelivery.delivery_type}</Text>
                  {codeDelivery.email_pattern && <Text>目标邮箱：{codeDelivery.email_pattern}</Text>}
                  {codeDelivery.fragment_url && <Text>验证入口：<a href={codeDelivery.fragment_url} target="_blank" rel="noreferrer">打开 Fragment</a></Text>}
                  {codeDelivery.timeout != null && <Text>等待时间：{codeDelivery.timeout} 秒</Text>}
                  {codeDelivery.next_label && <Text>未收到时 Telegram 提供的下一种方式：{codeDelivery.next_label}</Text>}
                  {codeDelivery.delivery_type === "SentCodeTypeApp" && <Text>Telegram 表示验证码应发送到其他已登录设备的官方服务通知。</Text>}
                  {!codeDelivery.next_label && <Text>当前响应没有提供下一种投递方式；不要连续重复申请，以免触发限流。</Text>}
                </Space>
              }
            />
          )}
          {codeSent && (
            <>
              <Input value={code} onChange={(event) => setCode(event.target.value)} placeholder="Telegram 登录验证码" />
              <Button type="primary" onClick={verifyCode} loading={busy}>验证登录验证码</Button>
            </>
          )}
          {needsPassword && (
            <>
              <Alert type="info" showIcon message="账号启用了两步验证，请输入 Telegram 2FA 密码。" />
              <Password value={password} onChange={(event) => setPassword(event.target.value)} placeholder="Telegram 两步验证密码" />
              <Button type="primary" onClick={verifyPassword} loading={busy}>完成两步验证</Button>
            </>
          )}
        </Space>
      </Card>

      <Card className="panel-card" title={<Space><CloudOutlined />云存储</Space>}>
        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label="rclone 状态">
            {settings?.rclone_configured ? <Tag color="success">已配置</Tag> : <Tag color="warning">未配置</Tag>}
          </Descriptions.Item>
          <Descriptions.Item label="可用 Remote">
            {settings?.rclone_remotes?.length ? settings.rclone_remotes.map((remote) => <Tag key={remote}>{remote}</Tag>) : "暂无 Remote"}
          </Descriptions.Item>
        </Descriptions>
        <Divider />
        <Title level={5}>默认备份目标</Title>
        <Paragraph type="secondary">格式为 remote:path。可以选择已有 Remote，也可以直接编辑目录路径。</Paragraph>
        {settings?.rclone_remotes?.length ? (
          <Select
            allowClear
            showSearch
            className="full-width"
            placeholder="选择一个已有 Remote"
            value={destination.split(":", 1)[0] || undefined}
            options={settings.rclone_remotes.map((remote) => ({ label: remote, value: remote }))}
            onChange={(remote) => setDestination(`${remote}:`)}
          />
        ) : null}
        <Input value={destination} onChange={(event) => setDestination(event.target.value)} placeholder="例如 backup:TelegramBackup" />
        <Space wrap>
          <Button icon={<CheckCircleOutlined />} onClick={testStorage} loading={busy} disabled={!settings?.rclone_configured}>测试连接</Button>
          <Button type="primary" icon={<SaveOutlined />} onClick={saveDestination} loading={busy} disabled={!settings?.rclone_configured}>设为默认备份目标</Button>
        </Space>
        <Divider />
        <Title level={5}>添加 Google Drive</Title>
        <Paragraph type="secondary">
          使用 Google Cloud OAuth 客户端和完整的 rclone authorize Token JSON。Token JSON 必须包含 refresh_token，系统会把整个 rclone 配置加密保存。
        </Paragraph>
        <Space direction="vertical" className="full-width">
          <Input value={remoteName} onChange={(event) => setRemoteName(event.target.value)} placeholder="Remote 名称，例如 gdrive" />
          <Input value={clientId} onChange={(event) => setClientId(event.target.value)} placeholder="OAuth Client ID" />
          <Password value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} placeholder="OAuth Client Secret" />
          <Input value={rootFolderId} onChange={(event) => setRootFolderId(event.target.value)} placeholder="Google Drive 文件夹 ID（可选）" />
          <TextArea
            value={tokenJson}
            onChange={(event) => setTokenJson(event.target.value)}
            rows={6}
            placeholder='粘贴 rclone authorize 输出的完整 JSON，例如 {"access_token":"...","refresh_token":"...","token_type":"Bearer","expiry":"..."}'
          />
          <Button type="primary" icon={<CloudOutlined />} loading={busy} onClick={addGoogleDrive} disabled={!settings?.encryption_ready}>
            添加并测试 Google Drive
          </Button>
        </Space>
      </Card>

      <Card className="panel-card" title={<Space><SettingOutlined />自动备份策略</Space>}>
        <Paragraph type="secondary">设置后台自动扫描已启用群组的时间间隔。修改后立即持久化，不需要重启服务。</Paragraph>
        <Space align="center" wrap>
          <Text>扫描间隔</Text>
          <InputNumber
            min={60}
            max={86400}
            step={60}
            value={scanInterval}
            onChange={(value) => setScanInterval(value || 300)}
          />
          <Text type="secondary">秒（最小 60 秒，最大 24 小时）</Text>
        </Space>
        <div style={{ marginTop: 16 }}>
          <Button type="primary" icon={<SaveOutlined />} onClick={saveBackup} loading={busy}>保存备份策略</Button>
        </div>
      </Card>

      <Card className="panel-card" title={<Space><LockOutlined />安全说明</Space>}>
        <Paragraph type="secondary">
          Telegram API Hash 和 rclone 配置通过 Fernet 加密后保存在应用数据库。Telegram Session 文件保存在服务器私有 data 卷中；任何人拿到 Session 都可能接管 Telegram 账号，请勿上传到 GitHub 或公开网盘。
        </Paragraph>
      </Card>
    </Space>
  );
}
