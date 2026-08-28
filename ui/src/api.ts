export interface Me {
  email: string;
  role: string;
}

export interface Device {
  id: string;
  hardware_id: string;
  display_name: string;
  status: string;
  last_known_state: string | null;
  last_seen_at: string | null;
  online: boolean;
  enrolled: boolean;
  enrolled_at: string | null;
  created_at: string;
}

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });

  if (res.status === 204) return null as T;

  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(res.status, body?.error ?? `HTTP ${res.status}`);
  }
  return body as T;
}

export const api = {
  me: () => request<Me>("/me"),

  login: (email: string, password: string) =>
    request<Me>("/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),

  logout: () => request<null>("/logout", { method: "POST" }),

  devices: () => request<Device[]>("/devices"),
    claim: (qrPayload: string, displayName: string) =>
    request<unknown>("/devices/claim", {
      method: "POST",
      body: JSON.stringify({ qr_payload: qrPayload, display_name: displayName }),
    }),

  release: (deviceId: string) =>
    request<unknown>(`/devices/${deviceId}/release`, { method: "POST" }),

  kirimPerintah: (deviceId: string, state: "ON" | "OFF") =>
    request<unknown>(`/devices/${deviceId}/command`, {
      method: "POST",
      body: JSON.stringify({ state }),
    }),
};