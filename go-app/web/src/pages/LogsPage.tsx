import { Card, CardTitle } from "../components/ui/Card";

export function LogsPage() {
  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">操作日志</h1>
        <p className="mt-1 text-sm text-muted-foreground">查看任务记录和审计日志。</p>
      </div>
      <Card>
        <CardTitle>任务记录 / 审计日志</CardTitle>
        <p className="mt-3 text-sm text-muted-foreground">这里将使用 Tab 展示历史任务摘要和操作审计。</p>
      </Card>
    </div>
  );
}
