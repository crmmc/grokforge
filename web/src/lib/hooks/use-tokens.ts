import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useRef, useState } from "react";
import { api, redirectToLogin } from "../api-client";
import type { Token, TokenUpdateRequest, PaginatedResponse } from "@/types";

export const tokenKeys = {
  all: ["tokens"] as const,
  lists: () => [...tokenKeys.all, "list"] as const,
  list: (params: {
    page?: number;
    page_size?: number;
    status?: string;
    nsfw?: boolean;
  }) => [...tokenKeys.lists(), params] as const,
  idsByStatus: (status: string | null) =>
    [...tokenKeys.all, "ids", status] as const,
  details: () => [...tokenKeys.all, "detail"] as const,
  detail: (id: number) => [...tokenKeys.details(), id] as const,
  stats: () => [...tokenKeys.all, "stats"] as const,
};

export function useTokens(
  params: {
    page?: number;
    page_size?: number;
    status?: string;
    nsfw?: boolean;
  } = {},
) {
  return useQuery({
    queryKey: tokenKeys.list(params),
    queryFn: () => api.get<PaginatedResponse<Token>>("/tokens", params),
  });
}

export function useToken(id: number | null) {
  return useQuery({
    queryKey: tokenKeys.detail(id ?? 0),
    queryFn: async () => {
      if (id === null) {
        throw new Error("token id is required");
      }
      return api.get<Token>(`/tokens/${id}`);
    },
    enabled: id !== null,
  });
}

export function useUpdateToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: TokenUpdateRequest }) =>
      api.put<Token>(`/tokens/${id}`, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: tokenKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: tokenKeys.lists() });
    },
  });
}

export function useDeleteToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.delete<void>(`/tokens/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tokenKeys.all });
    },
  });
}

export type BatchOperation =
  | "enable"
  | "disable"
  | "delete"
  | "enable_nsfw"
  | "export"
  | "import";

export interface BatchTokenRequest {
  operation: BatchOperation;
  ids?: number[];
  tokens?: string[];
  pool?: string;
  quotas?: Record<string, number>;
  priority?: number;
  status?: string;
  remark?: string;
  nsfw_enabled?: boolean;
  raw?: boolean;
}

export interface BatchTokenResponse {
  operation: string;
  success: number;
  failed: number;
  errors?: Array<{ index?: number; id?: number; message: string }>;
  tokens?: Token[];
  raw_tokens?: string[];
}

export function useBatchTokens() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: BatchTokenRequest) => {
      const endpoint = req.raw ? "/tokens/batch?raw=true" : "/tokens/batch";
      const { raw: _, ...body } = req;
      return api.post<BatchTokenResponse>(endpoint, body);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tokenKeys.all });
    },
  });
}

export function useRefreshToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.post<Token>(`/tokens/${id}/refresh`),
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: tokenKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: tokenKeys.lists() });
    },
  });
}

export function useTokenIdsByStatus(status: string | null) {
  return useQuery({
    queryKey: tokenKeys.idsByStatus(status),
    queryFn: () =>
      api.get<{ ids: number[] }>("/tokens/ids", status ? { status } : {}),
    enabled: status !== null,
  });
}

// --- Batch Refresh (SSE) ---

export interface BatchRefreshEvent {
  type: "progress" | "complete";
  token_id?: number;
  status?: "success" | "error";
  error?: string;
  current: number;
  total: number;
  success?: number;
  failed?: number;
}

export interface BatchRefreshCallbacks {
  onStart?: (total: number) => void;
  onProgress?: (event: BatchRefreshEvent) => void;
  onComplete?: (event: BatchRefreshEvent) => void;
  onCancel?: () => void;
  onError?: (error: Error) => void;
}

async function readBatchRefreshSSE(
  response: Response,
  callbacks: BatchRefreshCallbacks,
  signal: AbortSignal,
): Promise<void> {
  const reader = response.body?.getReader();
  if (!reader) throw new Error("No response body");

  const decoder = new TextDecoder();
  let buffer = "";
  let completed = false;

  try {
    for (;;) {
      if (signal.aborted) break;
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (!line.startsWith("data: ")) continue;
        const data = line.slice(6).trim();
        if (data === "[DONE]") continue;
        const event: BatchRefreshEvent = JSON.parse(data);
        if (event.type === "progress") {
          callbacks.onProgress?.(event);
        } else if (event.type === "complete") {
          completed = true;
          callbacks.onComplete?.(event);
        }
      }
    }
  } finally {
    reader.releaseLock();
  }

  if (!completed && !signal.aborted) {
    throw new Error("Refresh stream ended unexpectedly");
  }
}

export function useBatchRefresh() {
  const queryClient = useQueryClient();
  const [isRefreshing, setIsRefreshing] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const lockRef = useRef(false);

  const startRefresh = useCallback(
    async (
      ids: number[] | undefined,
      callbacks: BatchRefreshCallbacks,
    ) => {
      if (lockRef.current) return;
      lockRef.current = true;
      setIsRefreshing(true);
      callbacks.onStart?.(ids?.length ?? 0);

      const controller = new AbortController();
      abortRef.current = controller;

      try {
        const response = await fetch("/admin/tokens/batch/refresh", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ids }),
          signal: controller.signal,
        });

        if (!response.ok) {
          if (response.status === 401) redirectToLogin();
          const err = await response
            .json()
            .catch(() => ({ error: { message: "Unknown error" } }));
          throw new Error(
            err.error?.message || err.message || "Refresh failed",
          );
        }

        await readBatchRefreshSSE(response, callbacks, controller.signal);
      } catch (err) {
        if (err instanceof Error && err.name === "AbortError") {
          callbacks.onCancel?.();
        } else if (err instanceof Error) {
          callbacks.onError?.(err);
        }
      } finally {
        lockRef.current = false;
        setIsRefreshing(false);
        abortRef.current = null;
        queryClient.invalidateQueries({ queryKey: tokenKeys.all });
      }
    },
    [queryClient],
  );

  const cancel = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  return { startRefresh, cancel, isRefreshing };
}
