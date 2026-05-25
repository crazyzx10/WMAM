import { useEffect, useState } from "react";
import { Eye, RefreshCw, X } from "lucide-react";
import { Badge, toneForStatus } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { Card, CardTitle } from "../components/ui/Card";
import { EmptyState } from "../components/ui/EmptyState";
import { apiRequest } from "../lib/api";

type Job = {
  id: number;
  status: string;
  startedByUsername: string;
  startedAt: string;
  finishedAt?: string;
  totalPrograms: number;
  completedSteps: number;
  failedSteps: number;
  totalSteps: number;
  progressPercent: number;
  errorSummary?: string;
};

type AuditLog = {
  id: number;
  username?: string;
  action: string;
  description?: string;
  result: string;
  created_at?: string;
  createdAt?: string;
};

type JobStep = {
  id: number;
  programName: string;
  appIdMasked?: string;
  stepType: "adunit_list" | "summary" | "detail" | "settlement";
  status: string;
  recordCount: number;
  errorMessage?: string;
  startedAt?: string;
  finishedAt?: string;
};

type JobsResponse = {
  jobs: Job[];
  total: number;
};

type JobDetailResponse = {
  job: Job;
  steps: JobStep[];
};

type AuditResponse = {
  logs: AuditLog[];
  total: number;
};

const jobStatusLabels: Record<string, string> = {
  running: "执行中",
  interrupted: "已中断",
  failed: "失败",
  ended: "已结束",
  completed: "已完成"
};

const stepLabels: Record<JobStep["stepType"], string> = {
  adunit_list: "广告位",
  summary: "汇总",
  detail: "细分",
  settlement: "结算"
};

const stepStatusLabels: Record<string, string> = {
  pending: "待处理",
  running: "执行中",
  success: "成功",
  failed: "失败",
  skipped: "跳过"
};

function labelOf(labels: Record<string, string>, value?: string) {
  if (!value) {
    return "-";
  }
  return labels[value] ?? value;
}

export function LogsPage() {
  const [tab, setTab] = useState<"jobs" | "audit">("jobs");
  const [jobs, setJobs] = useState<Job[]>([]);
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [detail, setDetail] = useState<JobDetailResponse | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [error, setError] = useState("");

  async function loadData() {
    setError("");
    try {
      const [jobData, auditData] = await Promise.all([
        apiRequest<JobsResponse>("/api/jobs"),
        apiRequest<AuditResponse>("/api/audit-logs")
      ]);
      setJobs(jobData.jobs);
      setLogs(auditData.logs);
    } catch (err) {
      setError(err instanceof Error ? err.message : "获取日志失败");
    }
  }

  async function loadJobDetail(jobId: number) {
    setDetailError("");
    setDetailLoading(true);
    try {
      const data = await apiRequest<JobDetailResponse>(`/api/jobs/${jobId}`);
      setDetail(data);
    } catch (err) {
      setDetailError(err instanceof Error ? err.message : "获取任务详情失败");
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, []);

  return (
    <div className="space-y-5">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">操作日志</h1>
          <p className="mt-1 text-sm text-muted-foreground">查看历史任务摘要和操作审计日志。</p>
        </div>
        <Button variant="outline" onClick={loadData}>
          <RefreshCw className="h-4 w-4" />
          刷新
        </Button>
      </div>

      {error ? <div className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger">{error}</div> : null}

      <Card>
        <div className="mb-4 flex gap-2">
          <Button variant={tab === "jobs" ? "default" : "outline"} onClick={() => setTab("jobs")}>
            任务记录
          </Button>
          <Button variant={tab === "audit" ? "default" : "outline"} onClick={() => setTab("audit")}>
            审计日志
          </Button>
        </div>

        {tab === "jobs" ? (
          <div>
            <CardTitle>任务记录</CardTitle>
            <div className="mt-4 overflow-hidden rounded-md border border-border">
              <table className="w-full text-left text-sm">
                <thead className="bg-muted/60 text-muted-foreground">
                  <tr>
                    <th className="px-4 py-3 font-medium">任务</th>
                    <th className="px-4 py-3 font-medium">状态</th>
                    <th className="px-4 py-3 font-medium">发起人</th>
                    <th className="px-4 py-3 font-medium">进度</th>
                    <th className="px-4 py-3 font-medium">开始时间</th>
                    <th className="px-4 py-3 text-right font-medium">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {jobs.map((job) => (
                    <tr key={job.id} className="table-row">
                      <td className="px-4 py-3 font-medium">#{job.id}</td>
                      <td className="px-4 py-3">
                        <Badge tone={toneForStatus(job.status)}>{labelOf(jobStatusLabels, job.status)}</Badge>
                      </td>
                      <td className="px-4 py-3">{job.startedByUsername}</td>
                      <td className="px-4 py-3">
                        {job.completedSteps}/{job.totalSteps}，失败 {job.failedSteps}
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">{job.startedAt}</td>
                      <td className="px-4 py-3 text-right">
                        <Button variant="outline" size="sm" onClick={() => loadJobDetail(job.id)}>
                          <Eye className="h-4 w-4" />
                          查看
                        </Button>
                      </td>
                    </tr>
                  ))}
                  {jobs.length === 0 ? (
                    <tr>
                      <td colSpan={6}>
                        <EmptyState title="暂无任务记录" description="执行拉取后，这里会显示任务摘要和步骤详情入口。" />
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </div>
        ) : (
          <div>
            <CardTitle>审计日志</CardTitle>
            <div className="mt-4 overflow-hidden rounded-md border border-border">
              <table className="w-full text-left text-sm">
                <thead className="bg-muted/60 text-muted-foreground">
                  <tr>
                    <th className="px-4 py-3 font-medium">时间</th>
                    <th className="px-4 py-3 font-medium">用户</th>
                    <th className="px-4 py-3 font-medium">动作</th>
                    <th className="px-4 py-3 font-medium">说明</th>
                    <th className="px-4 py-3 font-medium">结果</th>
                  </tr>
                </thead>
                <tbody>
                  {logs.map((log) => (
                    <tr key={log.id} className="table-row">
                      <td className="px-4 py-3 text-muted-foreground">{log.created_at ?? log.createdAt}</td>
                      <td className="px-4 py-3">{log.username}</td>
                      <td className="px-4 py-3">{log.action}</td>
                      <td className="px-4 py-3">{log.description}</td>
                      <td className="px-4 py-3">
                        <Badge tone={log.result === "success" ? "success" : "danger"}>
                          {log.result === "success" ? "成功" : "失败"}
                        </Badge>
                      </td>
                    </tr>
                  ))}
                  {logs.length === 0 ? (
                    <tr>
                      <td colSpan={5}>
                        <EmptyState title="暂无审计日志" description="登录、配置、任务和用户操作会记录在这里。" />
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </Card>

      {detail || detailLoading || detailError ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/20 px-6 backdrop-blur-sm">
          <div className="max-h-[86vh] w-full max-w-[920px] overflow-hidden rounded-lg border border-border bg-card text-card-foreground shadow-xl">
            <div className="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
              <div>
                <h2 className="text-base font-semibold">任务详情{detail ? ` #${detail.job.id}` : ""}</h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  {detail
                    ? `${labelOf(jobStatusLabels, detail.job.status)} · 发起人 ${detail.job.startedByUsername} · ${detail.job.completedSteps}/${detail.job.totalSteps}`
                    : "正在加载任务详情"}
                </p>
              </div>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => {
                  setDetail(null);
                  setDetailError("");
                }}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>

            <div className="max-h-[calc(86vh-80px)] overflow-auto p-5">
              {detailError ? (
                <div className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger">{detailError}</div>
              ) : null}
              {detailLoading ? <div className="text-sm text-muted-foreground">加载中...</div> : null}
              {detail ? (
                <div className="space-y-4">
                  <div className="grid gap-3 text-sm sm:grid-cols-4">
                    <div>
                      <div className="text-muted-foreground">开始时间</div>
                      <div className="mt-1 font-medium">{detail.job.startedAt}</div>
                    </div>
                    <div>
                      <div className="text-muted-foreground">结束时间</div>
                      <div className="mt-1 font-medium">{detail.job.finishedAt || "-"}</div>
                    </div>
                    <div>
                      <div className="text-muted-foreground">小程序数</div>
                      <div className="mt-1 font-medium">{detail.job.totalPrograms}</div>
                    </div>
                    <div>
                      <div className="text-muted-foreground">进度</div>
                      <div className="mt-1 font-medium">{detail.job.progressPercent}%</div>
                    </div>
                  </div>

                  {detail.job.errorSummary ? (
                    <div className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger">
                      {detail.job.errorSummary}
                    </div>
                  ) : null}

                  <div className="overflow-hidden rounded-md border border-border">
                    <table className="w-full text-left text-sm">
                      <thead className="bg-muted/60 text-muted-foreground">
                        <tr>
                          <th className="px-4 py-3 font-medium">小程序</th>
                          <th className="px-4 py-3 font-medium">步骤</th>
                          <th className="px-4 py-3 font-medium">状态</th>
                          <th className="px-4 py-3 font-medium">记录数</th>
                          <th className="px-4 py-3 font-medium">错误</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.steps.map((step) => (
                          <tr key={step.id} className="table-row">
                            <td className="px-4 py-3">
                              <div className="font-medium">{step.programName}</div>
                              <div className="text-xs text-muted-foreground">{step.appIdMasked}</div>
                            </td>
                            <td className="px-4 py-3">{labelOf(stepLabels, step.stepType)}</td>
                            <td className="px-4 py-3">
                              <Badge tone={toneForStatus(step.status)}>{labelOf(stepStatusLabels, step.status)}</Badge>
                            </td>
                            <td className="px-4 py-3">{step.recordCount}</td>
                            <td className="max-w-[260px] px-4 py-3 text-danger">{step.errorMessage || "-"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
