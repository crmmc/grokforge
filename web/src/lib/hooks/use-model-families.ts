import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api-client'

// --- Types ---

export interface ModelFamily {
  id: number
  model: string
  display_name: string
  type: string
  enabled: boolean
  pool_floor: string
  default_mode_id: number | null
  upstream_model: string
  description: string
  created_at: string
  updated_at: string
}

export interface ModelMode {
  id: number
  model_id: number
  mode: string
  enabled: boolean
  pool_floor_override: string | null
  upstream_mode: string
  force_thinking: boolean
  enable_pro: boolean
  created_at: string
  updated_at: string
}

export interface FamilyWithModes extends ModelFamily {
  modes: ModelMode[]
}

// --- Query Keys ---

export const modelFamilyKeys = {
  all: ['model-families'] as const,
  lists: () => [...modelFamilyKeys.all, 'list'] as const,
}

// --- Queries ---

export function useModelFamilies() {
  return useQuery({
    queryKey: modelFamilyKeys.lists(),
    queryFn: () => api.get<FamilyWithModes[]>('/models/families'),
  })
}

// --- Family Mutations ---

function invalidateModelCaches(qc: ReturnType<typeof useQueryClient>) {
  qc.invalidateQueries({ queryKey: modelFamilyKeys.all })
  qc.invalidateQueries({ queryKey: ['models'] })
}

export function useCreateFamily() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<ModelFamily>) =>
      api.post<FamilyWithModes>('/models/families', data),
    onSuccess: () => invalidateModelCaches(qc),
  })
}

export function useUpdateFamily() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<ModelFamily> }) =>
      api.put<FamilyWithModes>(`/models/families/${id}`, data),
    onSuccess: () => invalidateModelCaches(qc),
  })
}

export function useDeleteFamily() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.delete<void>(`/models/families/${id}`),
    onSuccess: () => invalidateModelCaches(qc),
  })
}

// --- Mode Mutations ---

export function useCreateMode() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<ModelMode>) =>
      api.post<ModelMode>('/models/modes', data),
    onSuccess: () => invalidateModelCaches(qc),
  })
}

export function useUpdateMode() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<ModelMode> }) =>
      api.put<ModelMode>(`/models/modes/${id}`, data),
    onSuccess: () => invalidateModelCaches(qc),
  })
}

export function useDeleteMode() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.delete<void>(`/models/modes/${id}`),
    onSuccess: () => invalidateModelCaches(qc),
  })
}
