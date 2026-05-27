type ApiResponse<T> = {
  code: number;
  message: string;
  data?: T;
};

function canRetry(init: RequestInit) {
  const method = (init.method ?? "GET").toUpperCase();
  return method === "GET" || method === "HEAD";
}

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

async function fetchWithRetry(path: string, init: RequestInit) {
  try {
    return await fetch(path, init);
  } catch (err) {
    if (!canRetry(init)) {
      throw err;
    }
    await sleep(350);
    return fetch(path, init);
  }
}

export async function apiRequest<T>(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body && !(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  let response: Response;
  try {
    response = await fetchWithRetry(path, {
      ...init,
      headers,
      credentials: "same-origin"
    });
  } catch {
    throw new Error("网络请求失败，请确认 WMAM 服务正在运行后重试");
  }

  let payload: ApiResponse<T>;
  try {
    payload = (await response.json()) as ApiResponse<T>;
  } catch {
    throw new Error("服务器响应格式异常，请刷新页面后重试");
  }

  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || "请求失败");
  }

  return payload.data as T;
}
