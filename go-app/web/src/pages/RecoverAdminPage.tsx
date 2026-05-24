import { type FormEvent, useState } from "react";
import { ShieldCheck } from "lucide-react";
import { Link } from "react-router-dom";
import { Button } from "../components/ui/Button";
import { Card, CardTitle } from "../components/ui/Card";
import { apiRequest } from "../lib/api";

type RecoverResponse = {
  newRecoveryCode: string;
};

export function RecoverAdminPage() {
  const [recoveryCode, setRecoveryCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newRecoveryCode, setNewRecoveryCode] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setNewRecoveryCode("");
    setLoading(true);
    try {
      const data = await apiRequest<RecoverResponse>("/api/auth/recover-admin", {
        method: "POST",
        body: JSON.stringify({ recoveryCode, newPassword })
      });
      setNewRecoveryCode(data.newRecoveryCode);
      setRecoveryCode("");
      setNewPassword("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "重置失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6 text-foreground">
      <Card className="w-full max-w-[460px] p-8">
        <div className="mb-6 flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-md bg-muted">
            <ShieldCheck className="h-5 w-5" />
          </div>
          <div>
            <CardTitle>管理员恢复</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">恢复成功后旧恢复码会失效。</p>
          </div>
        </div>

        <form className="space-y-4" onSubmit={handleSubmit}>
          <input
            className="field"
            placeholder="恢复码"
            value={recoveryCode}
            onChange={(event) => setRecoveryCode(event.target.value)}
          />
          <input
            className="field"
            type="password"
            placeholder="新的管理员密码，至少 8 位"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
          />
          {error ? <div className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger">{error}</div> : null}
          {newRecoveryCode ? (
            <div className="rounded-md border border-success/30 bg-success/5 px-3 py-2 text-sm text-success">
              新恢复码：{newRecoveryCode}
            </div>
          ) : null}
          <Button className="w-full" disabled={loading || !recoveryCode || newPassword.length < 8}>
            {loading ? "重置中" : "重置管理员密码"}
          </Button>
        </form>

        <div className="mt-5 text-center text-sm text-muted-foreground">
          <Link to="/login" className="hover:text-foreground">
            返回登录
          </Link>
        </div>
      </Card>
    </div>
  );
}
