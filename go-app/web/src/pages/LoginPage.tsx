import { type FormEvent, useState } from "react";
import { Lock, Moon, Sun, User } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";
import { Button } from "../components/ui/Button";
import { Card } from "../components/ui/Card";
import { apiRequest } from "../lib/api";
import { type CurrentUser, setAuth } from "../lib/auth";
import { useTheme } from "../lib/theme";

type LoginResult = {
  expires_at: string;
  user: CurrentUser;
};

export function LoginPage() {
  const navigate = useNavigate();
  const { isDark, toggleTheme } = useTheme();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [rememberPassword, setRememberPassword] = useState(true);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setLoading(true);

    try {
      const data = await apiRequest<LoginResult>("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password, rememberPassword })
      });
      setAuth(data.user, rememberPassword);
      navigate(data.user.must_change_password ? "/change-password" : "/app/fetch", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6 text-foreground">
      <Button
        className="absolute right-6 top-6"
        variant="ghost"
        size="icon"
        type="button"
        onClick={toggleTheme}
        aria-label={isDark ? "切换浅色模式" : "切换暗色模式"}
        title={isDark ? "切换浅色模式" : "切换暗色模式"}
      >
        {isDark ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
      </Button>

      <Card className="w-full max-w-[420px] p-8">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-foreground font-semibold text-background shadow-[0_1px_1px_rgb(0_0_0/0.08),0_8px_16px_-10px_rgb(0_0_0/0.35)]">
            W
          </div>
          <h1 className="text-2xl font-semibold">WMAM</h1>
          <p className="mt-2 text-sm text-muted-foreground">微信小程序广告数据管理</p>
        </div>

        <form className="space-y-4" onSubmit={handleSubmit}>
          <label className="block space-y-2">
            <span className="text-sm text-muted-foreground">用户名</span>
            <div className="relative">
              <User className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <input
                className="field field-with-left-icon"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
              />
            </div>
          </label>

          <label className="block space-y-2">
            <span className="text-sm text-muted-foreground">密码</span>
            <div className="relative">
              <Lock className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <input
                className="field field-with-left-icon field-with-right-icon"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
              />
            </div>
          </label>

          <label className="flex items-center gap-2 text-sm text-muted-foreground">
            <input
              type="checkbox"
              className="h-4 w-4 rounded border-border"
              checked={rememberPassword}
              onChange={(event) => setRememberPassword(event.target.checked)}
            />
            记住密码
          </label>

          {error ? (
            <div className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger">
              {error}
            </div>
          ) : null}

          <Button className="w-full" disabled={loading || !username || !password}>
            {loading ? "登录中" : "登录"}
          </Button>
        </form>
        <div className="mt-5 text-center text-sm text-muted-foreground">
          <Link to="/recover-admin" className="hover:text-foreground">
            使用恢复码重置管理员密码
          </Link>
        </div>
      </Card>
    </div>
  );
}
