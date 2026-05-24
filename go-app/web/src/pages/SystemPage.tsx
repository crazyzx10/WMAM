import { type FormEvent, useEffect, useState } from "react";
import { DatabaseZap, RotateCcw, Save } from "lucide-react";
import { Button } from "../components/ui/Button";
import { Card, CardTitle } from "../components/ui/Card";
import { apiRequest } from "../lib/api";

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
  const [config, setConfig] = useState<MySQLConfig>(emptyConfig);
  const [adminPassword, setAdminPassword] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function loadConfig() {
    setError("");
    setLoading(true);
    try {
      const data = await apiRequest<MySQLResponse>("/api/system/mysql");
      setConfig({ ...emptyConfig, ...data.mysql, password: "" });
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
    setMessage("");
    setError("");
    try {
      await apiRequest("/api/system/mysql/test", {
        method: "POST",
        body: JSON.stringify(config)
      });
      setMessage("连接成功");
    } catch (err) {
      setError(err instanceof Error ? err.message : "连接失败");
    }
  }

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("");
    setError("");
    try {
      await apiRequest("/api/system/mysql", {
        method: "PUT",
        body: JSON.stringify({ ...config, adminPassword })
      });
      setConfig((current) => ({ ...current, password: "", passwordSet: true }));
      setAdminPassword("");
      setMessage("数据库配置已保存");
      await loadConfig();
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存失败");
    }
  }

  async function handleRestore() {
    if (!window.confirm("确认恢复上一份可用 MySQL 配置？")) {
      return;
    }
    setMessage("");
    setError("");
    try {
      await apiRequest("/api/system/mysql/restore-last-good", {
        method: "POST",
        body: JSON.stringify({ adminPassword })
      });
      setAdminPassword("");
      setMessage("已恢复上一份可用配置");
      await loadConfig();
    } catch (err) {
      setError(err instanceof Error ? err.message : "恢复失败");
    }
  }

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">系统配置</h1>
        <p className="mt-1 text-sm text-muted-foreground">配置广告数据 MySQL 连接和系统备份。WMAM 启动时不会自动连接 MySQL。</p>
      </div>

      {(message || error) && (
        <div
          className={[
            "rounded-md border px-3 py-2 text-sm",
            error ? "border-danger/30 bg-danger/5 text-danger" : "border-success/30 bg-success/5 text-success"
          ].join(" ")}
        >
          {error || message}
        </div>
      )}

      <Card>
        <div className="flex items-center justify-between gap-3">
          <CardTitle>数据库连接</CardTitle>
          <Button variant="outline" onClick={loadConfig} disabled={loading}>
            刷新
          </Button>
        </div>

        <form className="mt-4 grid grid-cols-2 gap-3" onSubmit={handleSave}>
          <input className="field" placeholder="MySQL 地址" value={config.host} onChange={(event) => update("host", event.target.value)} />
          <input
            className="field"
            type="number"
            placeholder="端口"
            value={config.port}
            onChange={(event) => update("port", Number(event.target.value))}
          />
          <input className="field" placeholder="数据库名" value={config.database} onChange={(event) => update("database", event.target.value)} />
          <input className="field" placeholder="用户名" value={config.username} onChange={(event) => update("username", event.target.value)} />
          <input
            className="field"
            type="password"
            placeholder={config.passwordSet ? "已设置，留空则不修改" : "数据库密码"}
            value={config.password ?? ""}
            onChange={(event) => update("password", event.target.value)}
          />
          <input
            className="field"
            type="password"
            placeholder="管理员密码，可留空"
            value={adminPassword}
            onChange={(event) => setAdminPassword(event.target.value)}
          />
          <div className="col-span-2 flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={handleRestore}>
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

      <Card>
        <CardTitle>系统备份</CardTitle>
        <p className="mt-3 text-sm text-muted-foreground">加密导出和导入覆盖会在后续备份阶段接入。</p>
      </Card>
    </div>
  );
}
