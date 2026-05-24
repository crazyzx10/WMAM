import {
  Database,
  FileClock,
  LogOut,
  Moon,
  Play,
  Smartphone,
  Sun,
  Users
} from "lucide-react";
import { NavLink, Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { apiRequest } from "../lib/api";
import { clearAuth, getStoredToken, getStoredUser } from "../lib/auth";
import { Button } from "../components/ui/Button";
import { FetchPage } from "../pages/FetchPage";
import { LogsPage } from "../pages/LogsPage";
import { LoginPage } from "../pages/LoginPage";
import { ProgramsPage } from "../pages/ProgramsPage";
import { SystemPage } from "../pages/SystemPage";
import { UsersPage } from "../pages/UsersPage";

const navItems = [
  { to: "/app/programs", label: "小程序配置", icon: Smartphone, adminOnly: true },
  { to: "/app/system", label: "系统配置", icon: Database, adminOnly: true },
  { to: "/app/fetch", label: "执行拉取", icon: Play },
  { to: "/app/logs", label: "操作日志", icon: FileClock },
  { to: "/app/users", label: "用户管理", icon: Users, adminOnly: true }
];

function ProtectedRoute() {
  const token = getStoredToken();
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return <AppLayout />;
}

function AppLayout() {
  const navigate = useNavigate();
  const user = getStoredUser();
  const visibleNav = navItems.filter((item) => !item.adminOnly || user?.role === "admin");

  async function handleLogout() {
    try {
      await apiRequest("/api/auth/logout", { method: "POST" });
    } finally {
      clearAuth();
      navigate("/login", { replace: true });
    }
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 z-20 flex w-64 flex-col border-r border-border bg-sidebar">
        <div className="flex h-16 items-center gap-3 border-b border-border px-5">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-foreground text-sm font-semibold text-background">
            W
          </div>
          <div>
            <div className="font-semibold tracking-tight">WMAM</div>
            <div className="text-xs text-muted-foreground">{user?.username ?? "未登录"}</div>
          </div>
        </div>
        <nav className="flex-1 space-y-1 px-3 py-4">
          {visibleNav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                [
                  "flex h-10 items-center gap-3 rounded-md px-3 text-sm transition",
                  isActive
                    ? "bg-muted font-medium text-foreground"
                    : "text-muted-foreground hover:bg-muted/70 hover:text-foreground"
                ].join(" ")
              }
            >
              <item.icon className="h-4 w-4" />
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <div className="pl-64">
        <header className="sticky top-0 z-10 border-b border-border bg-background/90 backdrop-blur">
          <div className="mx-auto flex h-16 max-w-[1080px] items-center justify-between px-6">
            <div>
              <div className="text-sm text-muted-foreground">微信小程序广告数据管理</div>
              <h1 className="text-lg font-semibold">多人控制台</h1>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="icon" aria-label="切换主题">
                <Sun className="h-4 w-4 dark:hidden" />
                <Moon className="hidden h-4 w-4 dark:block" />
              </Button>
              <Button variant="outline" onClick={handleLogout}>
                <LogOut className="mr-2 h-4 w-4" />
                退出登录
              </Button>
            </div>
          </div>
        </header>

        <main className="mx-auto max-w-[1080px] px-6 py-8">
          <Routes>
            <Route path="/fetch" element={<FetchPage />} />
            <Route path="/logs" element={<LogsPage />} />
            <Route path="/programs" element={<ProgramsPage />} />
            <Route path="/system" element={<SystemPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="*" element={<Navigate to="/app/fetch" replace />} />
          </Routes>
        </main>
      </div>
    </div>
  );
}

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/app/*" element={<ProtectedRoute />} />
      <Route path="*" element={<Navigate to="/app/fetch" replace />} />
    </Routes>
  );
}
