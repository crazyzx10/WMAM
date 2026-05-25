import { type FormEvent, useEffect, useState } from "react";
import { RefreshCw, Trash2, UserPlus } from "lucide-react";
import { Badge } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { Card, CardTitle } from "../components/ui/Card";
import { EmptyState } from "../components/ui/EmptyState";
import { PageHeader } from "../components/ui/PageHeader";
import { StatusMessage } from "../components/ui/StatusMessage";
import { TableShell } from "../components/ui/TableShell";
import { useToast } from "../components/ui/Toast";
import { apiRequest } from "../lib/api";

type UserRow = {
  id: number;
  username: string;
  role: "admin" | "user";
  status: "active" | "disabled";
  must_change_password: boolean;
  last_login_at?: string;
  created_at: string;
};

type UsersResponse = {
  users: UserRow[];
};

export function UsersPage() {
  const { toast } = useToast();
  const [users, setUsers] = useState<UserRow[]>([]);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [resetPasswordById, setResetPasswordById] = useState<Record<number, string>>({});
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function loadUsers() {
    setError("");
    setLoading(true);
    try {
      const data = await apiRequest<UsersResponse>("/api/users");
      setUsers(data.users);
    } catch (err) {
      setError(err instanceof Error ? err.message : "获取用户列表失败");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadUsers();
  }, []);

  async function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("");
    setError("");
    try {
      await apiRequest("/api/users", {
        method: "POST",
        body: JSON.stringify({ username, password })
      });
      setUsername("");
      setPassword("");
      setMessage("用户已创建");
      toast({ title: "用户已创建", variant: "success" });
      await loadUsers();
    } catch (err) {
      const message = err instanceof Error ? err.message : "创建用户失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleToggle(user: UserRow) {
    const nextStatus = user.status === "active" ? "disabled" : "active";
    if (nextStatus === "disabled" && !window.confirm(`确认禁用用户 ${user.username}？`)) {
      return;
    }
    setMessage("");
    setError("");
    try {
      await apiRequest(`/api/users/${user.id}`, {
        method: "PUT",
        body: JSON.stringify({ username: user.username, status: nextStatus })
      });
      setMessage(nextStatus === "active" ? "用户已启用" : "用户已禁用");
      toast({ title: nextStatus === "active" ? "用户已启用" : "用户已禁用", variant: "success" });
      await loadUsers();
    } catch (err) {
      const message = err instanceof Error ? err.message : "更新用户失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleResetPassword(user: UserRow) {
    const nextPassword = resetPasswordById[user.id] ?? "";
    setMessage("");
    setError("");
    try {
      await apiRequest(`/api/users/${user.id}/reset-password`, {
        method: "POST",
        body: JSON.stringify({ password: nextPassword })
      });
      setResetPasswordById((current) => ({ ...current, [user.id]: "" }));
      setMessage("密码已重置");
      toast({ title: "密码已重置", variant: "success" });
      await loadUsers();
    } catch (err) {
      const message = err instanceof Error ? err.message : "重置密码失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  async function handleDelete(user: UserRow) {
    if (!window.confirm(`确认删除用户 ${user.username}？`)) {
      return;
    }
    setMessage("");
    setError("");
    try {
      await apiRequest(`/api/users/${user.id}`, { method: "DELETE" });
      setMessage("用户已删除");
      toast({ title: "用户已删除", variant: "success" });
      await loadUsers();
    } catch (err) {
      const message = err instanceof Error ? err.message : "删除用户失败";
      setError(message);
      toast({ title: message, variant: "danger" });
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="用户管理"
        description="创建普通用户、重置密码、启用或禁用账号。"
        action={
          <Button variant="outline" onClick={loadUsers} disabled={loading}>
            <RefreshCw className={["h-4 w-4", loading ? "animate-spin" : ""].join(" ")} />
            刷新
          </Button>
        }
      />

      <Card>
        <CardTitle>创建普通用户</CardTitle>
        <form className="mt-4 grid gap-3 md:grid-cols-[1fr_1fr_auto]" onSubmit={handleCreate}>
          <input
            className="field"
            placeholder="用户名"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
          <input
            className="field"
            type="password"
            placeholder="初始密码，至少 8 位"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          <Button disabled={!username || password.length < 8}>
            <UserPlus className="h-4 w-4" />
            创建
          </Button>
        </form>
      </Card>

      <StatusMessage message={message} error={error} />

      <Card>
        <CardTitle>用户列表</CardTitle>
        <TableShell>
          <table className="min-w-full text-left text-sm">
            <thead className="bg-muted/60 text-muted-foreground">
              <tr>
                <th className="px-4 py-3 font-medium">用户名</th>
                <th className="px-4 py-3 font-medium">角色</th>
                <th className="px-4 py-3 font-medium">状态</th>
                <th className="px-4 py-3 font-medium">首次改密</th>
                <th className="px-4 py-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id} className="table-row">
                  <td className="max-w-[180px] break-words px-4 py-3 font-medium">{user.username}</td>
                  <td className="px-4 py-3">
                    <Badge tone={user.role === "admin" ? "neutral" : "warning"}>{user.role === "admin" ? "管理员" : "普通用户"}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <Badge tone={user.status === "active" ? "success" : "danger"}>{user.status === "active" ? "启用" : "禁用"}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <Badge tone={user.must_change_password ? "warning" : "success"}>{user.must_change_password ? "需要" : "不需要"}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    {user.role === "admin" ? (
                      <span className="text-muted-foreground">唯一管理员受保护</span>
                    ) : (
                      <div className="flex min-w-[420px] flex-wrap items-center gap-2">
                        <Button variant="outline" size="sm" onClick={() => handleToggle(user)}>
                          {user.status === "active" ? "禁用" : "启用"}
                        </Button>
                        <input
                          className="field field-xs w-40"
                          type="password"
                          placeholder="新密码"
                          value={resetPasswordById[user.id] ?? ""}
                          onChange={(event) =>
                            setResetPasswordById((current) => ({
                              ...current,
                              [user.id]: event.target.value
                            }))
                          }
                        />
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={(resetPasswordById[user.id] ?? "").length < 8}
                          onClick={() => handleResetPassword(user)}
                        >
                          重置密码
                        </Button>
                        <Button variant="danger" size="sm" onClick={() => handleDelete(user)}>
                          <Trash2 className="h-4 w-4" />
                          删除
                        </Button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
              {users.length === 0 ? (
                <tr>
                  <td colSpan={5}>
                    <EmptyState title="暂无用户" description="创建普通用户后，他们可以登录并执行自己的拉取任务。" />
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
