import { Card, CardTitle } from "../components/ui/Card";

export function SystemPage() {
  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">系统配置</h1>
        <p className="mt-1 text-sm text-muted-foreground">配置广告数据 MySQL 和系统备份。</p>
      </div>
      <Card>
        <CardTitle>数据库连接</CardTitle>
        <p className="mt-3 text-sm text-muted-foreground">密码不回显，留空则不修改。</p>
      </Card>
      <Card>
        <CardTitle>系统备份</CardTitle>
        <p className="mt-3 text-sm text-muted-foreground">导出和导入加密系统配置。</p>
      </Card>
    </div>
  );
}
