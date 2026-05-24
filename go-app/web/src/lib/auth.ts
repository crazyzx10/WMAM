export type CurrentUser = {
  id: number;
  username: string;
  role: "admin" | "user";
  must_change_password?: boolean;
};

const tokenKey = "wmam.auth.token";
const userKey = "wmam.auth.user";
const rememberKey = "wmam.auth.remember";

function activeStorage() {
  return localStorage.getItem(rememberKey) === "1" ? localStorage : sessionStorage;
}

export function getStoredToken() {
  return localStorage.getItem(tokenKey) ?? sessionStorage.getItem(tokenKey);
}

export function getStoredUser(): CurrentUser | null {
  const raw = activeStorage().getItem(userKey) ?? localStorage.getItem(userKey) ?? sessionStorage.getItem(userKey);
  if (!raw) {
    return null;
  }

  try {
    return JSON.parse(raw) as CurrentUser;
  } catch {
    clearAuth();
    return null;
  }
}

export function setAuth(token: string, user: CurrentUser, rememberPassword: boolean) {
  clearAuth();
  const storage = rememberPassword ? localStorage : sessionStorage;
  storage.setItem(tokenKey, token);
  storage.setItem(userKey, JSON.stringify(user));
  localStorage.setItem(rememberKey, rememberPassword ? "1" : "0");
}

export function clearAuth() {
  localStorage.removeItem(tokenKey);
  localStorage.removeItem(userKey);
  sessionStorage.removeItem(tokenKey);
  sessionStorage.removeItem(userKey);
  localStorage.removeItem(rememberKey);
}
