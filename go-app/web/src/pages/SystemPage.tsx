import { type FormEvent, useEffect, useState } from "react";
import { Copy, DatabaseZap, Download, RotateCcw, Save, ShieldCheck } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { Button } from "../components/ui/Button";
import { Card, CardTitle } from "../components/ui/Card";
import { PageHeader } from "../components/ui/PageHeader";
import { RefreshButton } from "../components/ui/RefreshButton";
import { StatusMessage } from "../components/ui/StatusMessage";
import { useToast } from "../components/ui/Toast";
import { apiRequest } from "../lib/api";
import { clearAuth } from "../lib/auth";

type MySQLConfig = {
  host: string;
  port: number;
  database: string;
  username: string;
  password?: string;
  passwordSet: boolean;
};

type MySQLResponse = {
  mysql: MySQLConfig;
  lastGoodAvailable: boolean;
};

type RecoveryCodeResponse = {
  recoveryCode: string;
};

const emptyConfig: MySQLConfig = {
  host: "",
  port: 3306,
  database: "",
  username: "",
  password: "",
  passwordSet: false
};

export function SystemPage() {
  const { toast } = useToast();
  const navigate = useNavigate();
  const [config, setConfig] = useState<MySQLConfig>(emptyConfig);
  const [lastGoodAvailable, setLastGoodAvailable] = useState(false);
  const [recoveryAdminPassword, setRecoveryAdminPassword] = useState("");
  const [backupAdminPassword, setBackupAdminPassword] = useState("");
  const [backupPassword, setBackupPassword] = useState("");
  const [backupFile, setBackupFile] = useState<File | null>(null);
  const [recoveryCode, setRecoveryCode] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function loadConfig() {
    setError("");
    setLoading(true);
    try {
      const data = await apiRequest<MySQLResponse>("/api/system/mysql");
      setConfig({ ...emptyConfig, ...data.mysql, password: "" });
      setLastGoodAvailable(data.lastGoodAvailable);
    } catch (err) {
      setError(err instanceof Error ? err.message : "读取配置失败");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadConfig();
  }, []);

  function update<K extends keyof MySQLConfig>(key: K, value: MySQLConfig[K]) {
    setConfig((current) => ({ ...current, [key]: value }));
  }

  async function handleTest() {
    setError("");
    try {
      await apiRequest("/api/system/mysql/test", {
        method: "POST",
        body: JSON.stringify(config)
      });
      toast({ title: "MySQL 连接成功", variant: "success" });
    } catch (err) {
      const message = err instanceof Error ? err.message : "连接失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    try {
      await apiRequest("/api/system/mysql", {
        method: "PUT",
        body: JSON.stringify(config)
      });
      setConfig((current) => ({ ...current, password: "", passwordSet: true }));
      toast({ title: "数据库配置已保存", variant: "success" });
      await loadConfig();
    } catch (err) {
      const message = err instanceof Error ? err.message : "保存失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleRestore() {
    if (!window.confirm("确认恢复上一份可用 MySQL 配置？")) {
      return;
    }
    setError("");
    try {
      await apiRequest("/api/system/mysql/restore-last-good", { method: "POST" });
      toast({ title: "已恢复上一份可用配置", variant: "success" });
      await loadConfig();
    } catch (err) {
      const message = err instanceof Error ? err.message : "恢复失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleExport() {
    setError("");
    try {
      const response = await fetch("/api/system/backup/export", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        credentials: "same-origin",
        body: JSON.stringify({ adminPassword: backupAdminPassword, backupPassword })
      });
      if (!response.ok) {
        let message = "导出失败";
        try {
          const payload = await response.json();
          message = payload.message || message;
        } catch {
          // keep the generic message
        }
        throw new Error(message);
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `wmam-backup-${Date.now()}.wmam`;
      link.click();
      URL.revokeObjectURL(url);
      toast({ title: "备份已导出", variant: "success" });
    } catch (err) {
      const message = err instanceof Error ? err.message : "导出失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleImport() {
    if (!backupFile) {
      setError("请选择备份文件");
      toast({ title: "请选择备份文件", variant: "danger" });
      return;
    }
    if (!window.confirm("导入会覆盖当前本地系统配置，确认继续？")) {
      return;
    }
    setError("");
    try {
      const form = new FormData();
      form.set("file", backupFile);
      form.set("backupPassword", backupPassword);
      form.set("adminPassword", backupAdminPassword);
      const response = await fetch("/api/system/backup/import", {
        method: "POST",
        credentials: "same-origin",
        body: form
      });
      const payload = await response.json();
      if (!response.ok || payload.code !== 0) {
        throw new Error(payload.message || "导入失败");
      }
      toast({ title: "备份已导入，请重新登录", variant: "success" });
      clearAuth();
      navigate("/login", { replace: true });
    } catch (err) {
      const message = err instanceof Error ? err.message : "导入失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleRotateRecoveryCode() {
    if (!window.confirm("生成新恢复码后，旧恢复码会立即失效，确认继续？")) {
      return;
    }
    setError("");
    setRecoveryCode("");
    try {
      const data = await apiRequest<RecoveryCodeResponse>("/api/system/recovery-code/rotate", {
        method: "POST",
        body: JSON.stringify({ adminPassword: recoveryAdminPassword })
      });
      setRecoveryCode(data.recoveryCode);
      setRecoveryAdminPassword("");
      toast({ title: "新恢复码已生成", variant: "success" });
    } catch (err) {
      const message = err instanceof Error ? err.message : "生成恢复码失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function copyRecoveryCode() {
    if (!recoveryCode) {
      return;
    }
    try {
      await navigator.clipboard.writeText(recoveryCode);
      toast({ title: "恢复码已复制", variant: "success" });
    } catch {
      toast({ title: "复制失败，请手动选择恢复码", variant: "danger" });
    }
  }

  function downloadRecoveryCode() {
    if (!recoveryCode) {
      return;
    }
    const blob = new Blob([`${recoveryCode}\n`], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `wmam-admin-recovery-${Date.now()}.txt`;
    link.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="space-y-5">
      <PageHeader title="系统配置" description="配置广告数据 MySQL 连接和系统备份。WMAM 启动时不会自动连接 MySQL。" />

      <StatusMessage error={error} />

      <Card>
        <div className="flex items-center justify-between gap-3">
          <CardTitle>数据库连接</CardTitle>
          <RefreshButton onClick={loadConfig} loading={loading} />
        </div>

        <form className="mt-4 space-y-3" onSubmit={handleSave}>
          <div className="grid gap-3 md:grid-cols-[minmax(0,1.4fr)_120px_minmax(0,1fr)]">
            <input className="field" placeholder="MySQL 地址" value={config.host} onChange={(event) => update("host", event.target.value)} />
            <input
              className="field"
              type="number"
              placeholder="端口"
              value={config.port}
              onChange={(event) => update("port", Number(event.target.value))}
            />
            <input className="field" placeholder="数据库名" value={config.database} onChange={(event) => update("database", event.target.value)} />
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <input className="field" placeholder="数据库账号" value={config.username} onChange={(event) => update("username", event.target.value)} />
            <input
              className="field"
              type="password"
              placeholder={config.passwordSet ? "已设置，留空则不修改" : "数据库密码"}
              value={config.password ?? ""}
              onChange={(event) => update("password", event.target.value)}
            />
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" disabled={!lastGoodAvailable} onClick={handleRestore}>
              <RotateCcw className="h-4 w-4" />
              恢复配置
            </Button>
            <Button type="button" variant="outline" onClick={handleTest}>
              <DatabaseZap className="h-4 w-4" />
              测试连接
            </Button>
            <Button disabled={!config.host || !config.database || !config.username}>
              <Save className="h-4 w-4" />
              保存配置
            </Button>
          </div>
        </form>
      </Card>

      <div className="grid gap-5 lg:grid-cols-2">
        <Card>
          <div className="flex items-start justify-between gap-3">
            <div>
              <CardTitle>管理员恢复码</CardTitle>
              <p className="mt-2 text-sm text-muted-foreground">恢复码只在生成后显示一次。重新生成会让旧恢复码失效。</p>
            </div>
            <ShieldCheck className="h-5 w-5 text-muted-foreground" />
          </div>
          <div className="mt-4 space-y-3">
            <input
              className="field"
              type="password"
              placeholder="管理员密码"
              value={recoveryAdminPassword}
              onChange={(event) => setRecoveryAdminPassword(event.target.value)}
            />
            <div className="flex justify-end">
              <Button type="button" variant="outline" disabled={!recoveryAdminPassword} onClick={handleRotateRecoveryCode}>
                生成新恢复码
              </Button>
            </div>
          </div>
          {recoveryCode ? (
            <div className="mt-4 rounded-md border border-border bg-muted/40 p-3">
              <div className="break-all font-mono text-sm">{recoveryCode}</div>
              <div className="mt-3 flex justify-end gap-2">
                <Button type="button" variant="outline" size="sm" onClick={copyRecoveryCode}>
                  <Copy className="h-4 w-4" />
                  复制
                </Button>
                <Button type="button" variant="outline" size="sm" onClick={downloadRecoveryCode}>
                  <Download className="h-4 w-4" />
                  下载
                </Button>
              </div>
            </div>
          ) : null}
        </Card>

        <Card>
          <CardTitle>系统备份</CardTitle>
          <p className="mt-3 text-sm text-muted-foreground">导出文件会整体加密；导入会覆盖当前本地系统配置。</p>
          <div className="mt-4 space-y-3">
            <input
              className="field"
              type="password"
              placeholder="管理员密码"
              value={backupAdminPassword}
              onChange={(event) => setBackupAdminPassword(event.target.value)}
            />
            <input
              className="field"
              type="password"
              placeholder="备份文件密码"
              value={backupPassword}
              onChange={(event) => setBackupPassword(event.target.value)}
            />
            <input className="field" type="file" accept=".wmam" onChange={(event) => setBackupFile(event.target.files?.[0] ?? null)} />
            <div className="flex flex-wrap justify-end gap-2">
              <Button type="button" variant="outline" disabled={!backupPassword || !backupAdminPassword} onClick={handleExport}>
                导出系统配置
              </Button>
              <Button type="button" variant="warning" disabled={!backupPassword || !backupFile || !backupAdminPassword} onClick={handleImport}>
                导入并覆盖
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
}
