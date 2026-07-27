import request from '@/config/axios'

export interface DashboardProject {
  id: number
  code: string
  name: string
  status: string
}

export interface DashboardMetrics {
  projects: number
  datasetVersions: number
  activeJobs: number
  models: number
  deployments: number
  gpuHours: number
}

export interface LifecycleStage {
  key: string
  label: string
  count: number
  blocked: number
  route: string
}

export interface DashboardTodo {
  key: string
  kind: 'ANNOTATION' | 'REVIEW' | 'APPROVAL' | 'FAILED_JOB' | 'ALERT'
  title: string
  reason: string
  resourceType: string
  resourceId: number
  projectId: number
  priority: 'URGENT' | 'HIGH' | 'NORMAL'
  route: string
  createTime: string
}

export interface DashboardJob {
  id: number
  projectId: number
  jobType: string
  resourceType: string
  resourceId: number
  status: string
  stage: string
  progress: number
  durationSeconds: number
  errorMessage: string
  remediation: string
  actions: Array<'VIEW' | 'CANCEL' | 'RETRY'>
  route: string
  createTime: string
}

export interface ServiceHealth {
  name: string
  provider: string
  status: string
  detail: string
  lastCheckedAt?: string
}

export interface DashboardActivity {
  id: number
  action: string
  resourceType: string
  resourceId: number
  projectId: number
  actorUserId: number
  createTime: string
}

export interface ProjectQuota {
  projectId: number
  maxConcurrentJobs: number
  monthlyGpuHours: number
  storageBytes: number
}

export interface ComputeNode {
  id: number
  name: string
  status: string
  gpuModel: string
  gpuCount: number
  gpuUtilization: number
  gpuMemoryBytes: number
  gpuUsedBytes: number
  cpuUtilization: number
  memoryBytes: number
  memoryUsedBytes: number
  lastHeartbeatAt: string
}

export interface DashboardSummary {
  scope: { projectId: number; projects: DashboardProject[] }
  metrics: DashboardMetrics
  jobs: { running: number; failed: number; pending: number }
  lifecycle: LifecycleStage[]
  todos: DashboardTodo[]
  recentJobs: DashboardJob[]
  services: ServiceHealth[]
  activities: DashboardActivity[]
  resources: {
    visibility: 'DETAIL' | 'QUOTA_ONLY'
    quotas: ProjectQuota[]
    storageBytes: number
    gpuHours: number
    nodes?: ComputeNode[]
  }
}

export const getDashboardSummary = (projectId?: number) =>
  request.get<DashboardSummary>({
    url: '/ai-platform/dashboard/summary',
    params: projectId ? { projectId } : undefined
  })

export const cancelDashboardJob = (jobId: number) =>
  request.post({ url: `/ai-platform/jobs/${jobId}/cancel` })

export const retryDashboardJob = (jobId: number) =>
  request.post({ url: `/ai-platform/jobs/${jobId}/retry` })
