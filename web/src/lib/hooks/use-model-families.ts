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
  quota_default: string | null
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
  quota_cost: number
  upstream_mode: string
  upstream_model: string
  quota_override: string | null
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

export function useCreateFamily() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<ModelFamily>) =>
      api.post<FamilyWithModes>('/models/families', data),
    onSuccess: () => qc.invalidateQueries({ queryKey: modelFamilyKeys.all }),
  })
}

export function useUpdateFamily() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<ModelFamily> }) =>
      api.put<FamilyWithModes>(`/models/families/${id}`, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: modelFamilyKeys.all }),
  })
}

export function useDeleteFamily() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.delete<void>(`/models/families/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: modelFamilyKeys.all }),
  })
}

// --- Mode Mutations ---

export function useCreateMode() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<ModelMode>) =>
      api.post<ModelMode>('/models/modes', data),
    onSuccess: () => qc.invalidateQueries({ queryKey: modelFamilyKeys.all }),
  })
}

export function useUpdateMode() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<ModelMode> }) =>
      api.put<ModelMode>(`/models/modes/${id}`, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: modelFamilyKeys.all }),
  })
}

export function useDeleteMode() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.delete<void>(`/models/modes/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: modelFamilyKeys.all }),
  })
}
