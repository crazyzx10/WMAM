import { Moon, Sun } from "lucide-react";
import { Button } from "../components/ui/Button";
import { Card } from "../components/ui/Card";

export function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6 text-foreground">
      <Button className="absolute right-6 top-6" variant="ghost" size="icon" aria-label="切换主题">
        <Sun className="h-4 w-4 dark:hidden" />
        <Moon className="hidden h-4 w-4 dark:block" />
      </Button>
      <Card className="w-full max-w-[420px] p-8">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-foreground font-semibold text-background">
            W
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">WMAM</h1>
          <p className="mt-2 text-sm text-muted-foreground">微信小程序广告数据管理</p>
        </div>
        <form className="space-y-4">
          <label className="block space-y-2">
            <span className="text-sm text-muted-foreground">用户名</span>
            <input className="field" placeholder="请输入用户名" />
          </label>
          <label className="block space-y-2">
            <span className="text-sm text-muted-foreground">密码</span>
            <input className="field" type="password" placeholder="请输入密码" />
          </label>
          <label className="flex items-center gap-2 text-sm text-muted-foreground">
            <input type="checkbox" className="h-4 w-4 rounded border-border" />
            记住密码
          </label>
          <Button className="w-full">登录</Button>
        </form>
      </Card>
    </div>
  );
}
