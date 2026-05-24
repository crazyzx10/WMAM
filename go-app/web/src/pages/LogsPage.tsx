import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "../components/ui/Button";
import { Card, CardTitle } from "../components/ui/Card";
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

type JobsResponse = {
  jobs: Job[];
  total: number;
};

type AuditResponse = {
  logs: AuditLog[];
  total: number;
};

export function LogsPage() {
  const [tab, setTab] = useState<"jobs" | "audit">("jobs");
  const [jobs, setJobs] = useState<Job[]>([]);
  const [logs, setLogs] = useState<AuditLog[]>([]);
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
                  </tr>
                </thead>
                <tbody>
                  {jobs.map((job) => (
                    <tr key={job.id} className="border-t border-border">
                      <td className="px-4 py-3 font-medium">#{job.id}</td>
                      <td className="px-4 py-3">{job.status}</td>
                      <td className="px-4 py-3">{job.startedByUsername}</td>
                      <td className="px-4 py-3">
                        {job.completedSteps}/{job.totalSteps}，失败 {job.failedSteps}
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">{job.startedAt}</td>
                    </tr>
                  ))}
                  {jobs.length === 0 ? (
                    <tr>
                      <td className="px-4 py-8 text-center text-muted-foreground" colSpan={5}>
                        暂无任务记录
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
                    <tr key={log.id} className="border-t border-border">
                      <td className="px-4 py-3 text-muted-foreground">{log.created_at ?? log.createdAt}</td>
                      <td className="px-4 py-3">{log.username}</td>
                      <td className="px-4 py-3">{log.action}</td>
                      <td className="px-4 py-3">{log.description}</td>
                      <td className="px-4 py-3">{log.result}</td>
                    </tr>
                  ))}
                  {logs.length === 0 ? (
                    <tr>
                      <td className="px-4 py-8 text-center text-muted-foreground" colSpan={5}>
                        暂无审计日志
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </Card>
    </div>
  );
}
