import { type FormEvent, useEffect, useState } from "react";
import { RefreshCw, Trash2 } from "lucide-react";
import { Button } from "../components/ui/Button";
import { Card, CardTitle } from "../components/ui/Card";
import { apiRequest } from "../lib/api";

type Program = {
  id: number;
  name: string;
  appId: string;
  appIdMasked: string;
  appSecretSet: boolean;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

type ProgramsResponse = {
  programs: Program[];
};

export function ProgramsPage() {
  const [programs, setPrograms] = useState<Program[]>([]);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [name, setName] = useState("");
  const [appId, setAppId] = useState("");
  const [appSecret, setAppSecret] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function loadPrograms() {
    setError("");
    setLoading(true);
    try {
      const data = await apiRequest<ProgramsResponse>("/api/programs");
      setPrograms(data.programs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "获取小程序失败");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadPrograms();
  }, []);

  function resetForm() {
    setEditingId(null);
    setName("");
    setAppId("");
    setAppSecret("");
    setEnabled(true);
  }

  function startEdit(program: Program) {
    setEditingId(program.id);
    setName(program.name);
    setAppId(program.appId);
    setAppSecret("");
    setEnabled(program.enabled);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("");
    setError("");
    try {
      if (editingId) {
        await apiRequest(`/api/programs/${editingId}`, {
          method: "PUT",
          body: JSON.stringify({ name, appSecret, enabled })
        });
        setMessage("小程序已更新");
      } else {
        await apiRequest("/api/programs", {
          method: "POST",
          body: JSON.stringify({ name, appId, appSecret, enabled })
        });
        setMessage("小程序已创建");
      }
      resetForm();
      await loadPrograms();
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存小程序失败");
    }
  }

  async function handleStatus(program: Program) {
    setMessage("");
    setError("");
    try {
      await apiRequest(`/api/programs/${program.id}/status`, {
        method: "POST",
        body: JSON.stringify({ enabled: !program.enabled })
      });
      setMessage(program.enabled ? "小程序已禁用" : "小程序已启用");
      await loadPrograms();
    } catch (err) {
      setError(err instanceof Error ? err.message : "更新状态失败");
    }
  }

  async function handleDelete(program: Program) {
    if (!window.confirm(`确认删除小程序 ${program.name}？`)) {
      return;
    }
    setMessage("");
    setError("");
    try {
      await apiRequest(`/api/programs/${program.id}`, { method: "DELETE" });
      setMessage("小程序已删除");
      await loadPrograms();
    } catch (err) {
      setError(err instanceof Error ? err.message : "删除小程序失败");
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">小程序配置</h1>
          <p className="mt-1 text-sm text-muted-foreground">维护参与拉取的微信小程序，AppSecret 只加密保存，不回显。</p>
        </div>
        <Button variant="outline" onClick={loadPrograms} disabled={loading}>
          <RefreshCw className="h-4 w-4" />
          刷新
        </Button>
      </div>

      <Card>
        <CardTitle>{editingId ? "编辑小程序" : "添加小程序"}</CardTitle>
        <form className="mt-4 grid grid-cols-2 gap-3" onSubmit={handleSubmit}>
          <input className="field" placeholder="小程序名称" value={name} onChange={(event) => setName(event.target.value)} />
          <input
            className="field"
            placeholder="AppID"
            value={appId}
            disabled={Boolean(editingId)}
            onChange={(event) => setAppId(event.target.value)}
          />
          <input
            className="field"
            type="password"
            placeholder={editingId ? "已设置，留空则不修改" : "AppSecret"}
            value={appSecret}
            onChange={(event) => setAppSecret(event.target.value)}
          />
          <label className="flex h-10 items-center gap-2 rounded-md border border-border px-3 text-sm text-muted-foreground">
            <input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
            启用
          </label>
          <div className="col-span-2 flex justify-end gap-2">
            {editingId ? (
              <Button type="button" variant="outline" onClick={resetForm}>
                取消
              </Button>
            ) : null}
            <Button disabled={!name || (!editingId && (!appId || !appSecret))}>{editingId ? "保存修改" : "添加小程序"}</Button>
          </div>
        </form>
      </Card>

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
        <CardTitle>小程序列表</CardTitle>
        <div className="mt-4 overflow-hidden rounded-md border border-border">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted/60 text-muted-foreground">
              <tr>
                <th className="px-4 py-3 font-medium">名称</th>
                <th className="px-4 py-3 font-medium">AppID</th>
                <th className="px-4 py-3 font-medium">AppSecret</th>
                <th className="px-4 py-3 font-medium">状态</th>
                <th className="px-4 py-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {programs.map((program) => (
                <tr key={program.id} className="border-t border-border">
                  <td className="px-4 py-3 font-medium">{program.name}</td>
                  <td className="px-4 py-3">{program.appIdMasked || program.appId}</td>
                  <td className="px-4 py-3">{program.appSecretSet ? "已设置" : "未设置"}</td>
                  <td className="px-4 py-3">{program.enabled ? "启用" : "禁用"}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-2">
                      <Button variant="outline" size="sm" onClick={() => startEdit(program)}>
                        编辑
                      </Button>
                      <Button variant="outline" size="sm" onClick={() => handleStatus(program)}>
                        {program.enabled ? "禁用" : "启用"}
                      </Button>
                      <Button variant="danger" size="sm" onClick={() => handleDelete(program)}>
                        <Trash2 className="h-4 w-4" />
                        删除
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
              {programs.length === 0 ? (
                <tr>
                  <td className="px-4 py-8 text-center text-muted-foreground" colSpan={5}>
                    暂无小程序
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
