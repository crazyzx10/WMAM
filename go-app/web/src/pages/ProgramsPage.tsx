import { Card, CardTitle } from "../components/ui/Card";

export function ProgramsPage() {
  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">小程序配置</h1>
        <p className="mt-1 text-sm text-muted-foreground">维护参与拉取的微信小程序。</p>
      </div>
      <Card>
        <CardTitle>小程序列表</CardTitle>
        <p className="mt-3 text-sm text-muted-foreground">后续接入添加、编辑、启用、禁用和删除。</p>
      </Card>
    </div>
  );
}
