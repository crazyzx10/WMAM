import { useState } from "react";
import {
  Database,
  FileClock,
  LogOut,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Play,
  Smartphone,
  Sun,
  Users
} from "lucide-react";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { apiRequest } from "../lib/api";
import { clearAuth, getStoredToken, getStoredUser } from "../lib/auth";
import { Button } from "../components/ui/Button";
import { useTheme } from "../lib/theme";
import { ChangePasswordPage } from "../pages/ChangePasswordPage";
import { FetchPage } from "../pages/FetchPage";
import { LogsPage } from "../pages/LogsPage";
import { LoginPage } from "../pages/LoginPage";
import { ProgramsPage } from "../pages/ProgramsPage";
import { RecoverAdminPage } from "../pages/RecoverAdminPage";
import { SystemPage } from "../pages/SystemPage";
import { UsersPage } from "../pages/UsersPage";

const navItems = [
  { to: "/app/programs", label: "小程序配置", icon: Smartphone, adminOnly: true },
  { to: "/app/system", label: "系统配置", icon: Database, adminOnly: true },
  { to: "/app/fetch", label: "执行拉取", icon: Play },
  { to: "/app/logs", label: "操作日志", icon: FileClock },
  { to: "/app/users", label: "用户管理", icon: Users, adminOnly: true }
];

const sidebarKey = "wmam.ui.sidebarCollapsed";

function ProtectedRoute() {
  const location = useLocation();
  const token = getStoredToken();
  const user = getStoredUser();
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  if (user?.must_change_password && location.pathname !== "/change-password") {
    return <Navigate to="/change-password" replace />;
  }
  return <AppLayout />;
}

function ProtectedChangePasswordRoute() {
  const token = getStoredToken();
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return <ChangePasswordPage />;
}

function AppLayout() {
  const navigate = useNavigate();
  const user = getStoredUser();
  const { isDark, toggleTheme } = useTheme();
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(() => localStorage.getItem(sidebarKey) === "1");
  const visibleNav = navItems.filter((item) => !item.adminOnly || user?.role === "admin");

  function toggleSidebar() {
    setIsSidebarCollapsed((current) => {
      const next = !current;
      localStorage.setItem(sidebarKey, next ? "1" : "0");
      return next;
    });
  }

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
      <aside
        className={[
          "fixed inset-y-0 left-0 z-20 flex flex-col border-r border-border bg-sidebar transition-[width]",
          isSidebarCollapsed ? "w-20" : "w-64"
        ].join(" ")}
      >
        <div
          className={[
            "flex h-16 items-center gap-3 border-b border-border px-5",
            isSidebarCollapsed ? "justify-center px-3" : ""
          ].join(" ")}
        >
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-foreground text-sm font-semibold text-background">
            W
          </div>
          <div className={isSidebarCollapsed ? "sr-only" : ""}>
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
                  isSidebarCollapsed ? "justify-center px-0" : "",
                  isActive
                    ? "bg-muted font-medium text-foreground"
                    : "text-muted-foreground hover:bg-muted/70 hover:text-foreground"
                ].join(" ")
              }
              title={isSidebarCollapsed ? item.label : undefined}
            >
              <item.icon className="h-4 w-4" />
              <span className={isSidebarCollapsed ? "sr-only" : ""}>{item.label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="border-t border-border p-3">
          <Button
            className="w-full"
            variant="ghost"
            size={isSidebarCollapsed ? "icon" : "default"}
            onClick={toggleSidebar}
            title={isSidebarCollapsed ? "展开侧边栏" : "折叠侧边栏"}
            aria-label={isSidebarCollapsed ? "展开侧边栏" : "折叠侧边栏"}
          >
            {isSidebarCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            <span className={isSidebarCollapsed ? "sr-only" : ""}>{isSidebarCollapsed ? "展开" : "折叠"}</span>
          </Button>
        </div>
      </aside>

      <div className={["transition-[padding-left]", isSidebarCollapsed ? "pl-20" : "pl-64"].join(" ")}>
        <header className="sticky top-0 z-10 border-b border-border bg-background/90 backdrop-blur">
          <div className="mx-auto flex h-16 max-w-[1080px] items-center justify-between px-6">
            <div>
              <div className="text-sm text-muted-foreground">微信小程序广告数据管理</div>
              <h1 className="text-lg font-semibold">多人控制台</h1>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="ghost"
                size="icon"
                onClick={toggleTheme}
                aria-label={isDark ? "切换浅色模式" : "切换暗色模式"}
                title={isDark ? "切换浅色模式" : "切换暗色模式"}
              >
                {isDark ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
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
      <Route path="/recover-admin" element={<RecoverAdminPage />} />
      <Route path="/change-password" element={<ProtectedChangePasswordRoute />} />
      <Route path="/app/*" element={<ProtectedRoute />} />
      <Route path="*" element={<Navigate to="/app/fetch" replace />} />
    </Routes>
  );
}
