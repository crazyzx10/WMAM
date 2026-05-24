import { useEffect, useMemo, useState } from "react";
import { Pause, Play, Square, StepForward } from "lucide-react";
import { Button } from "../components/ui/Button";
import { Card, CardHeader, CardTitle } from "../components/ui/Card";
import { apiRequest } from "../lib/api";

type Job = {
  id: number;
  status: "running" | "interrupted" | "failed" | "ended" | "completed";
  startedByUsername: string;
  currentProgramName?: string;
  currentStep?: string;
  progressPercent: number;
  completedSteps: number;
  failedSteps: number;
  totalSteps: number;
};

type JobStep = {
  id: number;
  programName: string;
  stepType: "adunit_list" | "summary" | "detail" | "settlement";
  status: "pending" | "running" | "success" | "failed" | "skipped";
};

type Permissions = {
  canStart: boolean;
  canInterrupt: boolean;
  canResume: boolean;
  canEnd: boolean;
};

type CurrentJobResponse = {
  job: Job | null;
  permissions: Permissions;
  steps: JobStep[];
};

type Program = {
  id: number;
  name: string;
  enabled: boolean;
};

type ProgramsResponse = {
  programs: Program[];
};

const emptyPermissions: Permissions = {
  canStart: true,
  canInterrupt: false,
  canResume: false,
  canEnd: false
};

const stepLabels: Record<JobStep["stepType"], string> = {
  adunit_list: "广告位",
  summary: "汇总",
  detail: "细分",
  settlement: "结算"
};

const statusLabels: Record<string, string> = {
  running: "执行中",
  interrupted: "已中断",
  failed: "失败",
  ended: "已结束",
  completed: "已完成",
  pending: "待处理",
  success: "成功",
  skipped: "跳过"
};

function stepMark(status?: string) {
  if (status === "success") return "✓";
  if (status === "running") return "●";
  if (status === "failed") return "×";
  if (status === "skipped") return "–";
  return "-";
}

export function FetchPage() {
  const [job, setJob] = useState<Job | null>(null);
  const [steps, setSteps] = useState<JobStep[]>([]);
  const [programs, setPrograms] = useState<Program[]>([]);
  const [permissions, setPermissions] = useState<Permissions>(emptyPermissions);
  const [logs, setLogs] = useState<string[]>(["页面刷新后实时日志会清空。"]);
  const [error, setError] = useState("");

  async function loadState() {
    setError("");
    try {
      const [jobData, programData] = await Promise.all([
        apiRequest<CurrentJobResponse>("/api/jobs/current"),
        apiRequest<ProgramsResponse>("/api/programs")
      ]);
      setJob(jobData.job);
      setSteps(jobData.steps);
      setPermissions(jobData.permissions);
      setPrograms(programData.programs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "获取任务状态失败");
    }
  }

  useEffect(() => {
    void loadState();
  }, []);

  useEffect(() => {
    if (job?.status !== "running") {
      return;
    }
    const timer = window.setInterval(() => {
      void loadState();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [job?.status]);

  const rows = useMemo(() => {
    const byProgram = new Map<string, Record<string, string>>();
    for (const step of steps) {
      const current = byProgram.get(step.programName) ?? {};
      current[step.stepType] = step.status;
      byProgram.set(step.programName, current);
    }

    return programs
      .filter((program) => program.enabled)
      .map((program) => {
        const stepState = byProgram.get(program.name) ?? {};
        const done = Object.values(stepState).filter((status) => status === "success").length;
        return {
          name: program.name,
          status: steps.length ? `${done}/4` : "待创建任务",
          adunit: stepMark(stepState.adunit_list),
          summary: stepMark(stepState.summary),
          detail: stepMark(stepState.detail),
          settlement: stepMark(stepState.settlement),
          progress: `${done}/4`
        };
      });
  }, [programs, steps]);

  async function runAction(action: "start" | "interrupt" | "resume" | "end") {
    setError("");
    try {
      if (action === "start") {
        const data = await apiRequest<{ job: Job }>("/api/jobs/start", { method: "POST" });
        setLogs((current) => [...current, `[${new Date().toLocaleTimeString()}] 已创建任务 #${data.job.id}`]);
      } else if (job) {
        await apiRequest(`/api/jobs/${job.id}/${action}`, { method: "POST" });
        setLogs((current) => [...current, `[${new Date().toLocaleTimeString()}] ${statusLabels[action] ?? action}任务 #${job.id}`]);
      }
      await loadState();
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败");
    }
  }

  const progress = job?.progressPercent ?? 0;

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">执行拉取</h1>
        <p className="mt-1 text-sm text-muted-foreground">手动拉取所有已启用小程序的广告数据。</p>
      </div>

      {error ? <div className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger">{error}</div> : null}

      <Card>
        <CardHeader>
          <div>
            <CardTitle>当前任务</CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              {job
                ? `${statusLabels[job.status]} · 发起人 ${job.startedByUsername} · ${job.currentProgramName || "等待执行"} / ${
                    stepLabels[job.currentStep as JobStep["stepType"]] ?? "等待步骤"
                  }`
                : "暂无任务"}
            </p>
          </div>
          <div className="text-sm font-medium">{progress}%</div>
        </CardHeader>
        <div className="h-2 overflow-hidden rounded-full bg-muted">
          <div className="h-full rounded-full bg-foreground transition-all" style={{ width: `${progress}%` }} />
        </div>
        <div className="mt-5 flex flex-wrap gap-3">
          <Button disabled={!permissions.canStart} onClick={() => runAction("start")}>
            <Play className="h-4 w-4" />
            开始拉取
          </Button>
          <Button variant="warning" disabled={!permissions.canInterrupt} onClick={() => runAction("interrupt")}>
            <Pause className="h-4 w-4" />
            中断拉取
          </Button>
          <Button variant="outline" disabled={!permissions.canResume} onClick={() => runAction("resume")}>
            <StepForward className="h-4 w-4" />
            继续拉取
          </Button>
          <Button variant="danger" disabled={!permissions.canEnd} onClick={() => window.confirm("确认结束当前任务？") && runAction("end")}>
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
              {rows.map((program) => (
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
              {rows.length === 0 ? (
                <tr>
                  <td className="px-4 py-8 text-center text-muted-foreground" colSpan={7}>
                    暂无已启用小程序
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>实时日志</CardTitle>
        </CardHeader>
        <div className="h-[360px] overflow-auto rounded-md border border-border bg-log p-4 font-mono text-sm leading-7 text-muted-foreground">
          {logs.map((line, index) => (
            <div key={`${line}-${index}`}>{line}</div>
          ))}
        </div>
      </Card>
    </div>
  );
}
