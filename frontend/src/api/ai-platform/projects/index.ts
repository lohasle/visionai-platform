import request from '@/config/axios'

export type ProjectStatus = 'DRAFT' | 'ACTIVE' | 'SUSPENDED' | 'ARCHIVED'

export interface Project {
  id: number
  code: string
  name: string
  description: string
  aiDomain: string
  taskType: string
  defaultProvider: string
  storageBucket: string
  status: ProjectStatus
  ownerUserId: number
  createTime: string
  updateTime: string
}

export interface ProjectCreatePayload {
  code: string
  name: string
  description: string
  aiDomain: string
  taskType: string
  ownerUserId?: number
  defaultProvider: string
  storageBucket: string
  maxConcurrentJobs: number
  monthlyGpuHours: number
  storageBytes: number
}

export interface ProjectMember {
  id: number
  userId: number
  username: string
  nickname: string
  userStatus: number
  roles: ProjectMemberRole[]
  createTime: string
}

export interface ProjectMemberRole {
  id: number
  name: string
  code: string
  status: number
}

export interface ProjectConfig {
  storageConfig: Record<string, unknown>
  providerConfig: Record<string, unknown>
  secretRefs: Record<string, string>
  updateTime: string
}

export interface ProjectPage {
  list: Project[]
  total: number
}

export interface ProjectOverview {
  project: Project
  metrics: {
    assets: number
    annotations: number
    datasetVersions: number
    trainingRuns: number
    modelVersions: number
    deployments: number
  }
  risks: Array<{
    code: string
    severity: 'HIGH' | 'MEDIUM' | 'LOW'
    count: number
    message: string
    route: string
  }>
  timeline: Array<{
    id: number
    action: string
    resourceType: string
    resourceId: number
    actorUserId: number
    createTime: string
  }>
  quota: {
    maxConcurrentJobs: number
    monthlyGpuHours: number
    storageBytes: number
  }
  config: ProjectConfig
}

export const getProjectPage = (params: Record<string, unknown>) =>
  request.get<ProjectPage>({ url: '/ai-platform/projects', params })

export const createProject = (data: ProjectCreatePayload) =>
  request.post<Project>({ url: '/ai-platform/projects', data })

export const updateProjectStatus = (id: number, status: ProjectStatus) =>
  request.put<Project>({ url: `/ai-platform/projects/${id}/status`, data: { status } })

export const archiveProject = (id: number) =>
  request.post({ url: `/ai-platform/projects/${id}/archive` })

export const cloneProject = (id: number, data: Pick<Project, 'code' | 'name' | 'description'>) =>
  request.post<Project>({ url: `/ai-platform/projects/${id}/clone`, data })

export const getProjectMembers = (id: number) =>
  request.get<ProjectMember[]>({ url: `/ai-platform/projects/${id}/members` })

export const getProjectOverview = (id: number) =>
  request.get<ProjectOverview>({ url: `/ai-platform/projects/${id}/overview` })

export const upsertProjectMember = (id: number, data: { userId: number }) =>
  request.put({ url: `/ai-platform/projects/${id}/members`, data })

export const removeProjectMember = (id: number, userId: number) =>
  request.delete({ url: `/ai-platform/projects/${id}/members/${userId}` })

export const getProjectConfig = (id: number) =>
  request.get<ProjectConfig>({ url: `/ai-platform/projects/${id}/config` })

export const updateProjectConfig = (
  id: number,
  data: Omit<ProjectConfig, 'updateTime'> & { changeReason: string }
) => request.put<ProjectConfig>({ url: `/ai-platform/projects/${id}/config`, data })
