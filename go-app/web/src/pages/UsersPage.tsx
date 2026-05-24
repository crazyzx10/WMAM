import { Card, CardTitle } from "../components/ui/Card";

export function UsersPage() {
  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">用户管理</h1>
        <p className="mt-1 text-sm text-muted-foreground">创建普通用户、重置密码、启用或禁用账号。</p>
      </div>
      <Card>
        <CardTitle>用户列表</CardTitle>
        <p className="mt-3 text-sm text-muted-foreground">唯一管理员不可删除、禁用或降级。</p>
      </Card>
    </div>
  );
}
