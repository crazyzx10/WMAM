import { useEffect, useState } from "react";
import {
  CheckCircle2,
  Circle,
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
import { clearAuth, getStoredUser, setAuth, shouldPersistAuth, type CurrentUser } from "../lib/auth";
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
const setupDismissKey = "wmam.ui.setupDismissed";

type SetupState = {
  loaded: boolean;
  dismissed: boolean;
  mysqlConfigured: boolean;
  programConfigured: boolean;
};

type MySQLSetupResponse = {
  mysql: {
    host?: string;
    database?: string;
    username?: string;
    passwordSet?: boolean;
  };
};

type ProgramSetupResponse = {
  programs: Array<{
    id: number;
    enabled: boolean;
  }>;
};

type AuthMeResponse = {
  user: CurrentUser;
};

function AuthLoading() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background text-sm text-muted-foreground">
      WMAM
    </div>
  );
}

function useAuthUser() {
  const [state, setState] = useState<{ loading: boolean; user: CurrentUser | null }>(() => {
    const user = getStoredUser();
    return { loading: !user, user };
  });

  useEffect(() => {
    if (state.user) {
      return;
    }

    let active = true;
    async function loadCurrentUser() {
      try {
        const data = await apiRequest<AuthMeResponse>("/api/auth/me");
        if (!active) {
          return;
        }
        setAuth(data.user, shouldPersistAuth());
        setState({ loading: false, user: data.user });
      } catch {
        clearAuth();
        if (active) {
          setState({ loading: false, user: null });
        }
      }
    }

    void loadCurrentUser();
    return () => {
      active = false;
    };
  }, [state.user]);

  return state;
}

function ProtectedRoute() {
  const location = useLocation();
  const { loading, user } = useAuthUser();
  if (loading) {
    return <AuthLoading />;
  }
  if (!user) {
    return <Navigate to="/login" replace />;
  }
  if (user?.must_change_password && location.pathname !== "/change-password") {
    return <Navigate to="/change-password" replace />;
  }
  return <AppLayout />;
}

function ProtectedChangePasswordRoute() {
  const { loading, user } = useAuthUser();
  if (loading) {
    return <AuthLoading />;
  }
  if (!user) {
    return <Navigate to="/login" replace />;
  }
  return <ChangePasswordPage />;
}

function SetupStep({ done, label }: { done: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2 text-sm">
      {done ? <CheckCircle2 className="h-4 w-4 text-success" /> : <Circle className="h-4 w-4 text-muted-foreground" />}
      <span className={done ? "text-muted-foreground" : "font-medium"}>{label}</span>
    </div>
  );
}

function AppLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const user = getStoredUser();
  const { isDark, toggleTheme } = useTheme();
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(() => localStorage.getItem(sidebarKey) === "1");
  const [setup, setSetup] = useState<SetupState>(() => ({
    loaded: false,
    dismissed: localStorage.getItem(setupDismissKey) === "1",
    mysqlConfigured: false,
    programConfigured: false
  }));
  const visibleNav = navItems.filter((item) => !item.adminOnly || user?.role === "admin");
  const showSetupGuide =
    user?.role === "admin" && setup.loaded && !setup.dismissed && (!setup.mysqlConfigured || !setup.programConfigured);

  useEffect(() => {
    if (user?.role !== "admin" || setup.dismissed) {
      return;
    }

    let active = true;
    async function loadSetupState() {
      try {
        const [mysqlData, programData] = await Promise.all([
          apiRequest<MySQLSetupResponse>("/api/system/mysql"),
          apiRequest<ProgramSetupResponse>("/api/programs")
        ]);
        if (!active) {
          return;
        }
        const mysql = mysqlData.mysql;
        setSetup((current) => ({
          ...current,
          loaded: true,
          mysqlConfigured: Boolean(mysql.host && mysql.database && mysql.username && mysql.passwordSet),
          programConfigured: programData.programs.some((program) => program.enabled)
        }));
      } catch {
        if (active) {
          setSetup((current) => ({ ...current, loaded: true }));
        }
      }
    }

    void loadSetupState();
    return () => {
      active = false;
    };
  }, [location.pathname, setup.dismissed, user?.role]);

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
          {showSetupGuide ? (
            <div className="mb-5 rounded-lg border border-border bg-card p-5 text-card-foreground">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-base font-semibold">初始化检查</h2>
                  <p className="mt-1 text-sm text-muted-foreground">完成这些项目后，WMAM 就可以正式执行拉取。</p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    localStorage.setItem(setupDismissKey, "1");
                    setSetup((current) => ({ ...current, dismissed: true }));
                  }}
                >
                  收起
                </Button>
              </div>
              <div className="mt-4 grid gap-3 md:grid-cols-3">
                <SetupStep done={!user?.must_change_password} label="管理员密码已修改" />
                <SetupStep done={setup.mysqlConfigured} label="MySQL 已配置" />
                <SetupStep done={setup.programConfigured} label="小程序已启用" />
              </div>
              <div className="mt-4 flex flex-wrap gap-2">
                {!setup.mysqlConfigured ? (
                  <Button variant="outline" size="sm" onClick={() => navigate("/app/system")}>
                    去配置 MySQL
                  </Button>
                ) : null}
                {!setup.programConfigured ? (
                  <Button variant="outline" size="sm" onClick={() => navigate("/app/programs")}>
                    去添加小程序
                  </Button>
                ) : null}
              </div>
            </div>
          ) : null}
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
