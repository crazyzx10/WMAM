import { type FormEvent, useEffect, useState } from "react";
import { Trash2 } from "lucide-react";
import { Badge } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { Card, CardTitle } from "../components/ui/Card";
import { EmptyState } from "../components/ui/EmptyState";
import { PageHeader } from "../components/ui/PageHeader";
import { RefreshButton } from "../components/ui/RefreshButton";
import { StatusMessage } from "../components/ui/StatusMessage";
import { TableShell } from "../components/ui/TableShell";
import { useToast } from "../components/ui/Toast";
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
  const { toast } = useToast();
  const [programs, setPrograms] = useState<Program[]>([]);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [name, setName] = useState("");
  const [appId, setAppId] = useState("");
  const [appSecret, setAppSecret] = useState("");
  const [enabled, setEnabled] = useState(true);
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
    setError("");
    try {
      if (editingId) {
        await apiRequest(`/api/programs/${editingId}`, {
          method: "PUT",
          body: JSON.stringify({ name, appSecret, enabled })
        });
        toast({ title: "小程序已更新", variant: "success" });
      } else {
        await apiRequest("/api/programs", {
          method: "POST",
          body: JSON.stringify({ name, appId, appSecret, enabled })
        });
        toast({ title: "小程序已创建", variant: "success" });
      }
      resetForm();
      await loadPrograms();
    } catch (err) {
      const message = err instanceof Error ? err.message : "保存小程序失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleStatus(program: Program) {
    if (program.enabled && !window.confirm(`确认禁用小程序 ${program.name}？`)) {
      return;
    }
    setError("");
    try {
      await apiRequest(`/api/programs/${program.id}/status`, {
        method: "POST",
        body: JSON.stringify({ enabled: !program.enabled })
      });
      toast({ title: program.enabled ? "小程序已禁用" : "小程序已启用", variant: "success" });
      await loadPrograms();
    } catch (err) {
      const message = err instanceof Error ? err.message : "更新状态失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleDelete(program: Program) {
    if (!window.confirm(`确认删除小程序 ${program.name}？`)) {
      return;
    }
    setError("");
    try {
      await apiRequest(`/api/programs/${program.id}`, { method: "DELETE" });
      toast({ title: "小程序已删除", variant: "success" });
      await loadPrograms();
    } catch (err) {
      const message = err instanceof Error ? err.message : "删除小程序失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="小程序配置"
        description="维护参与拉取的微信小程序，AppSecret 只加密保存，不回显。"
        action={<RefreshButton onClick={loadPrograms} loading={loading} />}
      />

      <Card>
        <CardTitle>{editingId ? "编辑小程序" : "添加小程序"}</CardTitle>
        <form className="mt-4 grid gap-3 md:grid-cols-2" onSubmit={handleSubmit}>
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
          <div className="flex justify-end gap-2 md:col-span-2">
            {editingId ? (
              <Button type="button" variant="outline" onClick={resetForm}>
                取消
              </Button>
            ) : null}
            <Button disabled={!name || (!editingId && (!appId || !appSecret))}>{editingId ? "保存修改" : "添加小程序"}</Button>
          </div>
        </form>
      </Card>

      <StatusMessage error={error} />

      <Card>
        <CardTitle>小程序列表</CardTitle>
        <TableShell>
          <table className="min-w-full text-left text-sm">
            <thead className="bg-muted/45 text-muted-foreground">
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
                <tr key={program.id} className="table-row">
                  <td className="max-w-[220px] break-words px-4 py-3 font-medium">{program.name}</td>
                  <td className="break-all px-4 py-3 font-mono text-xs">{program.appIdMasked || program.appId}</td>
                  <td className="px-4 py-3">
                    <Badge tone={program.appSecretSet ? "success" : "warning"}>{program.appSecretSet ? "已设置" : "未设置"}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <Badge tone={program.enabled ? "success" : "neutral"}>{program.enabled ? "启用" : "禁用"}</Badge>
                  </td>
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
                  <td colSpan={5}>
                    <EmptyState title="暂无小程序" description="添加并启用小程序后，执行拉取页会按这些配置运行任务。" />
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </TableShell>
      </Card>
    </div>
  );
}
