import request from '@/config/axios'

export interface JobSummary {
  running: number
  failed: number
  pending: number
}

export interface LifecycleStage {
  key: string
  label: string
  count: number
}

export interface ServiceHealth {
  name: string
  status: string
}

export interface DashboardSummary {
  jobs: JobSummary
  lifecycle: LifecycleStage[]
  services: ServiceHealth[]
}

export const getDashboardSummary = () =>
  request.get<DashboardSummary>({ url: '/ai-platform/dashboard/summary' })
