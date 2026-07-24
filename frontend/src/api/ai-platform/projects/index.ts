import request from '@/config/axios'

export type ProjectStatus = 'DRAFT' | 'ACTIVE' | 'SUSPENDED' | 'ARCHIVED'

export interface Project {
  id: number
  code: string
  name: string
  description: string
  status: ProjectStatus
  ownerUserId: number
  createTime: string
  updateTime: string
}

export interface ProjectMember {
  id: number
  userId: number
  roles: string[]
  createTime: string
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

export const getProjectPage = (params: Record<string, unknown>) =>
  request.get<ProjectPage>({ url: '/ai-platform/projects', params })

export const createProject = (data: Pick<Project, 'code' | 'name' | 'description'>) =>
  request.post<Project>({ url: '/ai-platform/projects', data })

export const updateProjectStatus = (id: number, status: ProjectStatus) =>
  request.put<Project>({ url: `/ai-platform/projects/${id}/status`, data: { status } })

export const archiveProject = (id: number) =>
  request.post({ url: `/ai-platform/projects/${id}/archive` })

export const cloneProject = (id: number, data: Pick<Project, 'code' | 'name' | 'description'>) =>
  request.post<Project>({ url: `/ai-platform/projects/${id}/clone`, data })

export const getProjectMembers = (id: number) =>
  request.get<ProjectMember[]>({ url: `/ai-platform/projects/${id}/members` })

export const upsertProjectMember = (id: number, data: { userId: number; roles: string[] }) =>
  request.put({ url: `/ai-platform/projects/${id}/members`, data })

export const removeProjectMember = (id: number, userId: number) =>
  request.delete({ url: `/ai-platform/projects/${id}/members/${userId}` })

export const getProjectConfig = (id: number) =>
  request.get<ProjectConfig>({ url: `/ai-platform/projects/${id}/config` })

export const updateProjectConfig = (id: number, data: Omit<ProjectConfig, 'updateTime'>) =>
  request.put<ProjectConfig>({ url: `/ai-platform/projects/${id}/config`, data })
