import { Pause, Play, Square, StepForward } from "lucide-react";
import { Button } from "../components/ui/Button";
import { Card, CardHeader, CardTitle } from "../components/ui/Card";

const programs = [
  { name: "小程序 A", status: "已完成", adunit: "✓", summary: "✓", detail: "✓", settlement: "✓", progress: "4/4" },
  { name: "小程序 B", status: "执行中", adunit: "✓", summary: "●", detail: "-", settlement: "-", progress: "2/4" },
  { name: "小程序 C", status: "待处理", adunit: "-", summary: "-", detail: "-", settlement: "-", progress: "0/4" },
  { name: "小程序 D", status: "失败", adunit: "✓", summary: "✕", detail: "-", settlement: "-", progress: "1/4" }
];

export function FetchPage() {
  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">执行拉取</h1>
        <p className="mt-1 text-sm text-muted-foreground">手动拉取所有已启用小程序的广告数据。</p>
      </div>

      <Card>
        <CardHeader>
          <div>
            <CardTitle>当前任务</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">执行中 · 发起人 admin · 小程序 B / 汇总数据</p>
          </div>
          <div className="text-sm font-medium">65%</div>
        </CardHeader>
        <div className="h-2 overflow-hidden rounded-full bg-muted">
          <div className="h-full w-[65%] rounded-full bg-foreground transition-all" />
        </div>
        <div className="mt-5 flex flex-wrap gap-3">
          <Button>
            <Play className="h-4 w-4" />
            开始拉取
          </Button>
          <Button variant="warning">
            <Pause className="h-4 w-4" />
            中断拉取
          </Button>
          <Button variant="outline">
            <StepForward className="h-4 w-4" />
            继续拉取
          </Button>
          <Button variant="danger">
            <Square className="h-4 w-4" />
            结束拉取
          </Button>
        </div>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>小程序状态</CardTitle>
        </CardHeader>
        <div className="overflow-hidden rounded-md border border-border">
          <table className="w-full border-collapse text-sm">
            <thead className="bg-muted/60 text-left text-muted-foreground">
              <tr>
                <th className="px-4 py-3 font-medium">小程序名称</th>
                <th className="px-4 py-3 font-medium">总状态</th>
                <th className="px-4 py-3 font-medium">广告位</th>
                <th className="px-4 py-3 font-medium">汇总</th>
                <th className="px-4 py-3 font-medium">细分</th>
                <th className="px-4 py-3 font-medium">结算</th>
                <th className="px-4 py-3 font-medium">进度</th>
              </tr>
            </thead>
            <tbody>
              {programs.map((program) => (
                <tr key={program.name} className="border-t border-border">
                  <td className="px-4 py-3 font-medium">{program.name}</td>
                  <td className="px-4 py-3 text-muted-foreground">{program.status}</td>
                  <td className="px-4 py-3">{program.adunit}</td>
                  <td className="px-4 py-3">{program.summary}</td>
                  <td className="px-4 py-3">{program.detail}</td>
                  <td className="px-4 py-3">{program.settlement}</td>
                  <td className="px-4 py-3 text-muted-foreground">{program.progress}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>实时日志</CardTitle>
        </CardHeader>
        <div className="h-[360px] overflow-auto rounded-md border border-border bg-log p-4 font-mono text-sm leading-7 text-muted-foreground">
          <div>[10:30:00] 系统准备就绪</div>
          <div>[10:30:01] 开始拉取小程序数据...</div>
          <div className="text-success">[10:30:05] ✓ 小程序 A 广告位清单已保存</div>
          <div>[10:30:11] 小程序 B 汇总数据拉取中...</div>
        </div>
      </Card>
    </div>
  );
}
